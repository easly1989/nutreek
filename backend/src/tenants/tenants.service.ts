import { Injectable } from '@nestjs/common';
import { PrismaService } from '../prisma.service';

export interface CreateTenantDto {
  name: string;
}

export interface InviteMemberDto {
  email: string;
  role: 'admin' | 'member';
}

@Injectable()
export class TenantsService {
  constructor(private prisma: PrismaService) {}

  async create(createTenantDto: CreateTenantDto, userId: string) {
    const tenant = await this.prisma.tenant.create({
      data: {
        name: createTenantDto.name,
      },
    });

    await this.prisma.membership.create({
      data: {
        userId,
        tenantId: tenant.id,
        role: 'admin',
      },
    });

    return tenant;
  }

  async findAll(userId: string) {
    return this.prisma.tenant.findMany({
      where: {
        memberships: {
          some: {
            userId,
          },
        },
      },
      include: {
        memberships: true,
      },
    });
  }

  async invite(tenantId: string, inviteMemberDto: InviteMemberDto) {
    // Find user by email
    const user = await this.prisma.user.findUnique({
      where: { email: inviteMemberDto.email },
    });

    if (!user) {
      throw new Error('User not found');
    }

    // Check if membership already exists
    const existingMembership = await this.prisma.membership.findUnique({
      where: {
        userId_tenantId: {
          userId: user.id,
          tenantId,
        },
      },
    });

    if (existingMembership) {
      throw new Error('User is already a member of this tenant');
    }

    return this.prisma.membership.create({
      data: {
        userId: user.id,
        tenantId,
        role: inviteMemberDto.role,
      },
    });
  }

  async findOne(id: string) {
    return this.prisma.tenant.findUnique({
      where: { id },
      include: {
        memberships: {
          include: {
            user: true,
          },
        },
      },
    });
  }
}