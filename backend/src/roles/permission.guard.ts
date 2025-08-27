import { Injectable, CanActivate, ExecutionContext, ForbiddenException } from '@nestjs/common';
import { Reflector } from '@nestjs/core';
import { RolesService } from './roles.service';

@Injectable()
export class PermissionGuard implements CanActivate {
  constructor(
    private reflector: Reflector,
    private rolesService: RolesService,
  ) {}

  async canActivate(context: ExecutionContext): Promise<boolean> {
    const requiredPermission = this.reflector.get<string>('permission', context.getHandler());

    if (!requiredPermission) {
      return true; // No permission required
    }

    const request = context.switchToHttp().getRequest();
    const userId = request.user?.id;
    const tenantId = request.params?.tenantId || request.body?.tenantId || request.query?.tenantId;

    if (!userId || !tenantId) {
      throw new ForbiddenException('User ID and Tenant ID are required');
    }

    const [resource, action] = requiredPermission.split(':');

    const hasPermission = await this.rolesService.hasPermission(userId, tenantId, resource, action);

    if (!hasPermission) {
      throw new ForbiddenException(`Insufficient permissions: ${requiredPermission}`);
    }

    return true;
  }
}