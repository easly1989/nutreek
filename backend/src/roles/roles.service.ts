import { Injectable, NotFoundException, BadRequestException } from '@nestjs/common';
import { PrismaService } from '../prisma.service';

@Injectable()
export class RolesService {
  constructor(private prisma: PrismaService) {}

  async createRole(data: { name: string; description?: string; permissionIds?: string[] }) {
    const { permissionIds, ...roleData } = data;

    return this.prisma.role.create({
      data: {
        ...roleData,
        permissions: permissionIds ? {
          connect: permissionIds.map(id => ({ id }))
        } : undefined
      },
      include: {
        permissions: true
      }
    });
  }

  async getAllRoles() {
    return this.prisma.role.findMany({
      include: {
        permissions: true,
        _count: {
          select: { memberships: true }
        }
      }
    });
  }

  async getRoleById(id: string) {
    const role = await this.prisma.role.findUnique({
      where: { id },
      include: {
        permissions: true,
        memberships: {
          include: {
            user: {
              select: { id: true, name: true, email: true }
            }
          }
        }
      }
    });

    if (!role) {
      throw new NotFoundException('Role not found');
    }

    return role;
  }

  async updateRole(id: string, data: { name?: string; description?: string; permissionIds?: string[] }) {
    const { permissionIds, ...roleData } = data;

    return this.prisma.role.update({
      where: { id },
      data: {
        ...roleData,
        permissions: permissionIds ? {
          set: permissionIds.map(id => ({ id }))
        } : undefined
      },
      include: {
        permissions: true
      }
    });
  }

  async deleteRole(id: string) {
    const role = await this.getRoleById(id);

    if (role.isSystem) {
      throw new BadRequestException('Cannot delete system roles');
    }

    // Check if role is being used
    const membershipCount = await this.prisma.membership.count({
      where: { roleId: id }
    });

    if (membershipCount > 0) {
      throw new BadRequestException('Cannot delete role that is assigned to users');
    }

    return this.prisma.role.delete({
      where: { id }
    });
  }

  async assignRoleToUser(userId: string, tenantId: string, roleId: string) {
    // Verify role exists
    await this.getRoleById(roleId);

    // Update membership with role
    return this.prisma.membership.update({
      where: {
        userId_tenantId: {
          userId,
          tenantId
        }
      },
      data: { roleId },
      include: {
        role: {
          include: {
            permissions: true
          }
        }
      }
    });
  }

  async removeRoleFromUser(userId: string, tenantId: string) {
    return this.prisma.membership.update({
      where: {
        userId_tenantId: {
          userId,
          tenantId
        }
      },
      data: { roleId: null }
    });
  }

  async getUserPermissions(userId: string, tenantId: string): Promise<string[]> {
    const membership = await this.prisma.membership.findUnique({
      where: {
        userId_tenantId: {
          userId,
          tenantId
        }
      },
      include: {
        role: {
          include: {
            permissions: true
          }
        }
      }
    });

    if (!membership?.role) {
      return [];
    }

    return membership.role.permissions.map(p => `${p.resource}:${p.action}`);
  }

  async hasPermission(userId: string, tenantId: string, resource: string, action: string): Promise<boolean> {
    const permissions = await this.getUserPermissions(userId, tenantId);
    return permissions.includes(`${resource}:${action}`);
  }

  async createPermission(data: { name: string; resource: string; action: string; description?: string }) {
    return this.prisma.permission.create({
      data
    });
  }

  async getAllPermissions() {
    return this.prisma.permission.findMany({
      orderBy: [
        { resource: 'asc' },
        { action: 'asc' }
      ]
    });
  }

  async getPermissionsByResource(resource: string) {
    return this.prisma.permission.findMany({
      where: { resource },
      orderBy: { action: 'asc' }
    });
  }

  async deletePermission(id: string) {
    // Check if permission is being used by any roles
    const roleCount = await this.prisma.role.count({
      where: {
        permissions: {
          some: { id }
        }
      }
    });

    if (roleCount > 0) {
      throw new BadRequestException('Cannot delete permission that is assigned to roles');
    }

    return this.prisma.permission.delete({
      where: { id }
    });
  }

  async initializeSystemRoles() {
    const systemRoles = [
      {
        name: 'admin',
        description: 'Full administrative access',
        permissions: [
          'user:create', 'user:read', 'user:update', 'user:delete',
          'tenant:create', 'tenant:read', 'tenant:update', 'tenant:delete',
          'role:create', 'role:read', 'role:update', 'role:delete',
          'recipe:create', 'recipe:read', 'recipe:update', 'recipe:delete',
          'meal:create', 'meal:read', 'meal:update', 'meal:delete',
          'plan:create', 'plan:read', 'plan:update', 'plan:delete',
          'shopping-list:create', 'shopping-list:read', 'shopping-list:update', 'shopping-list:delete',
          'analytics:read', 'collaboration:create', 'collaboration:read', 'collaboration:update', 'collaboration:delete'
        ]
      },
      {
        name: 'manager',
        description: 'Management access with limited administrative capabilities',
        permissions: [
          'user:read', 'user:update',
          'tenant:read', 'tenant:update',
          'recipe:create', 'recipe:read', 'recipe:update', 'recipe:delete',
          'meal:create', 'meal:read', 'meal:update', 'meal:delete',
          'plan:create', 'plan:read', 'plan:update', 'plan:delete',
          'shopping-list:create', 'shopping-list:read', 'shopping-list:update', 'shopping-list:delete',
          'analytics:read', 'collaboration:create', 'collaboration:read', 'collaboration:update', 'collaboration:delete'
        ]
      },
      {
        name: 'member',
        description: 'Standard user access',
        permissions: [
          'recipe:create', 'recipe:read', 'recipe:update',
          'meal:create', 'meal:read', 'meal:update',
          'plan:create', 'plan:read', 'plan:update',
          'shopping-list:read', 'shopping-list:update',
          'analytics:read', 'collaboration:create', 'collaboration:read', 'collaboration:update'
        ]
      },
      {
        name: 'viewer',
        description: 'Read-only access',
        permissions: [
          'recipe:read', 'meal:read', 'plan:read', 'shopping-list:read', 'analytics:read', 'collaboration:read'
        ]
      }
    ];

    // Create permissions first
    const permissions = [
      // User permissions
      { name: 'user:create', resource: 'user', action: 'create', description: 'Create users' },
      { name: 'user:read', resource: 'user', action: 'read', description: 'View users' },
      { name: 'user:update', resource: 'user', action: 'update', description: 'Update users' },
      { name: 'user:delete', resource: 'user', action: 'delete', description: 'Delete users' },

      // Tenant permissions
      { name: 'tenant:create', resource: 'tenant', action: 'create', description: 'Create tenants' },
      { name: 'tenant:read', resource: 'tenant', action: 'read', description: 'View tenants' },
      { name: 'tenant:update', resource: 'tenant', action: 'update', description: 'Update tenants' },
      { name: 'tenant:delete', resource: 'tenant', action: 'delete', description: 'Delete tenants' },

      // Role permissions
      { name: 'role:create', resource: 'role', action: 'create', description: 'Create roles' },
      { name: 'role:read', resource: 'role', action: 'read', description: 'View roles' },
      { name: 'role:update', resource: 'role', action: 'update', description: 'Update roles' },
      { name: 'role:delete', resource: 'role', action: 'delete', description: 'Delete roles' },

      // Recipe permissions
      { name: 'recipe:create', resource: 'recipe', action: 'create', description: 'Create recipes' },
      { name: 'recipe:read', resource: 'recipe', action: 'read', description: 'View recipes' },
      { name: 'recipe:update', resource: 'recipe', action: 'update', description: 'Update recipes' },
      { name: 'recipe:delete', resource: 'recipe', action: 'delete', description: 'Delete recipes' },

      // Meal permissions
      { name: 'meal:create', resource: 'meal', action: 'create', description: 'Create meals' },
      { name: 'meal:read', resource: 'meal', action: 'read', description: 'View meals' },
      { name: 'meal:update', resource: 'meal', action: 'update', description: 'Update meals' },
      { name: 'meal:delete', resource: 'meal', action: 'delete', description: 'Delete meals' },

      // Plan permissions
      { name: 'plan:create', resource: 'plan', action: 'create', description: 'Create plans' },
      { name: 'plan:read', resource: 'plan', action: 'read', description: 'View plans' },
      { name: 'plan:update', resource: 'plan', action: 'update', description: 'Update plans' },
      { name: 'plan:delete', resource: 'plan', action: 'delete', description: 'Delete plans' },

      // Shopping list permissions
      { name: 'shopping-list:create', resource: 'shopping-list', action: 'create', description: 'Create shopping lists' },
      { name: 'shopping-list:read', resource: 'shopping-list', action: 'read', description: 'View shopping lists' },
      { name: 'shopping-list:update', resource: 'shopping-list', action: 'update', description: 'Update shopping lists' },
      { name: 'shopping-list:delete', resource: 'shopping-list', action: 'delete', description: 'Delete shopping lists' },

      // Analytics permissions
      { name: 'analytics:read', resource: 'analytics', action: 'read', description: 'View analytics' },

      // Collaboration permissions
      { name: 'collaboration:create', resource: 'collaboration', action: 'create', description: 'Create collaboration items' },
      { name: 'collaboration:read', resource: 'collaboration', action: 'read', description: 'View collaboration items' },
      { name: 'collaboration:update', resource: 'collaboration', action: 'update', description: 'Update collaboration items' },
      { name: 'collaboration:delete', resource: 'collaboration', action: 'delete', description: 'Delete collaboration items' },
    ];

    // Create permissions
    for (const permission of permissions) {
      await this.prisma.permission.upsert({
        where: { name: permission.name },
        update: {},
        create: permission
      });
    }

    // Create roles with permissions
    for (const role of systemRoles) {
      const permissionRecords = await this.prisma.permission.findMany({
        where: {
          name: { in: role.permissions }
        }
      });

      await this.prisma.role.upsert({
        where: { name: role.name },
        update: {},
        create: {
          name: role.name,
          description: role.description,
          isSystem: true,
          permissions: {
            connect: permissionRecords.map(p => ({ id: p.id }))
          }
        }
      });
    }

    return { message: 'System roles and permissions initialized successfully' };
  }
}