import {
  Controller,
  Get,
  Post,
  Put,
  Delete,
  Body,
  Param,
  Query,
  HttpCode,
  HttpStatus
} from '@nestjs/common';
import { ApiTags, ApiOperation, ApiResponse } from '@nestjs/swagger';
import { RolesService } from './roles.service';

@ApiTags('roles')
@Controller('roles')
export class RolesController {
  constructor(private readonly rolesService: RolesService) {}

  @Post()
  @ApiOperation({ summary: 'Create a new role' })
  @ApiResponse({ status: 201, description: 'Role created successfully' })
  async createRole(@Body() data: { name: string; description?: string; permissionIds?: string[] }) {
    return this.rolesService.createRole(data);
  }

  @Get()
  @ApiOperation({ summary: 'Get all roles' })
  @ApiResponse({ status: 200, description: 'Roles retrieved successfully' })
  async getAllRoles() {
    return this.rolesService.getAllRoles();
  }

  @Get(':id')
  @ApiOperation({ summary: 'Get role by ID' })
  @ApiResponse({ status: 200, description: 'Role retrieved successfully' })
  async getRoleById(@Param('id') id: string) {
    return this.rolesService.getRoleById(id);
  }

  @Put(':id')
  @ApiOperation({ summary: 'Update role' })
  @ApiResponse({ status: 200, description: 'Role updated successfully' })
  async updateRole(
    @Param('id') id: string,
    @Body() data: { name?: string; description?: string; permissionIds?: string[] }
  ) {
    return this.rolesService.updateRole(id, data);
  }

  @Delete(':id')
  @ApiOperation({ summary: 'Delete role' })
  @ApiResponse({ status: 200, description: 'Role deleted successfully' })
  async deleteRole(@Param('id') id: string) {
    return this.rolesService.deleteRole(id);
  }

  @Post('user/:userId/tenant/:tenantId/role/:roleId')
  @ApiOperation({ summary: 'Assign role to user in tenant' })
  @ApiResponse({ status: 200, description: 'Role assigned successfully' })
  async assignRoleToUser(
    @Param('userId') userId: string,
    @Param('tenantId') tenantId: string,
    @Param('roleId') roleId: string
  ) {
    return this.rolesService.assignRoleToUser(userId, tenantId, roleId);
  }

  @Delete('user/:userId/tenant/:tenantId/role')
  @ApiOperation({ summary: 'Remove role from user in tenant' })
  @ApiResponse({ status: 200, description: 'Role removed successfully' })
  async removeRoleFromUser(
    @Param('userId') userId: string,
    @Param('tenantId') tenantId: string
  ) {
    return this.rolesService.removeRoleFromUser(userId, tenantId);
  }

  @Get('user/:userId/tenant/:tenantId/permissions')
  @ApiOperation({ summary: 'Get user permissions in tenant' })
  @ApiResponse({ status: 200, description: 'Permissions retrieved successfully' })
  async getUserPermissions(
    @Param('userId') userId: string,
    @Param('tenantId') tenantId: string
  ) {
    const permissions = await this.rolesService.getUserPermissions(userId, tenantId);
    return { permissions };
  }

  @Get('user/:userId/tenant/:tenantId/check-permission')
  @ApiOperation({ summary: 'Check if user has specific permission' })
  @ApiResponse({ status: 200, description: 'Permission check result' })
  async checkPermission(
    @Param('userId') userId: string,
    @Param('tenantId') tenantId: string,
    @Query('resource') resource: string,
    @Query('action') action: string
  ) {
    const hasPermission = await this.rolesService.hasPermission(userId, tenantId, resource, action);
    return { hasPermission };
  }

  @Post('permissions')
  @ApiOperation({ summary: 'Create a new permission' })
  @ApiResponse({ status: 201, description: 'Permission created successfully' })
  async createPermission(@Body() data: { name: string; resource: string; action: string; description?: string }) {
    return this.rolesService.createPermission(data);
  }

  @Get('permissions')
  @ApiOperation({ summary: 'Get all permissions' })
  @ApiResponse({ status: 200, description: 'Permissions retrieved successfully' })
  async getAllPermissions(@Query('resource') resource?: string) {
    if (resource) {
      return this.rolesService.getPermissionsByResource(resource);
    }
    return this.rolesService.getAllPermissions();
  }

  @Delete('permissions/:id')
  @ApiOperation({ summary: 'Delete permission' })
  @ApiResponse({ status: 200, description: 'Permission deleted successfully' })
  async deletePermission(@Param('id') id: string) {
    return this.rolesService.deletePermission(id);
  }

  @Post('initialize')
  @HttpCode(HttpStatus.OK)
  @ApiOperation({ summary: 'Initialize system roles and permissions' })
  @ApiResponse({ status: 200, description: 'System roles initialized successfully' })
  async initializeSystemRoles() {
    return this.rolesService.initializeSystemRoles();
  }
}