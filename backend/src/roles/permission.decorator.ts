import { SetMetadata } from '@nestjs/common';

export const PERMISSION_KEY = 'permission';
export const Permission = (permission: string) => SetMetadata(PERMISSION_KEY, permission);

// Convenience decorators for common permissions
export const UserPermissions = {
  CREATE: Permission('user:create'),
  READ: Permission('user:read'),
  UPDATE: Permission('user:update'),
  DELETE: Permission('user:delete'),
};

export const TenantPermissions = {
  CREATE: Permission('tenant:create'),
  READ: Permission('tenant:read'),
  UPDATE: Permission('tenant:update'),
  DELETE: Permission('tenant:delete'),
};

export const RecipePermissions = {
  CREATE: Permission('recipe:create'),
  READ: Permission('recipe:read'),
  UPDATE: Permission('recipe:update'),
  DELETE: Permission('recipe:delete'),
};

export const MealPermissions = {
  CREATE: Permission('meal:create'),
  READ: Permission('meal:read'),
  UPDATE: Permission('meal:update'),
  DELETE: Permission('meal:delete'),
};

export const PlanPermissions = {
  CREATE: Permission('plan:create'),
  READ: Permission('plan:read'),
  UPDATE: Permission('plan:update'),
  DELETE: Permission('plan:delete'),
};

export const ShoppingListPermissions = {
  CREATE: Permission('shopping-list:create'),
  READ: Permission('shopping-list:read'),
  UPDATE: Permission('shopping-list:update'),
  DELETE: Permission('shopping-list:delete'),
};

export const AnalyticsPermissions = {
  READ: Permission('analytics:read'),
};

export const CollaborationPermissions = {
  CREATE: Permission('collaboration:create'),
  READ: Permission('collaboration:read'),
  UPDATE: Permission('collaboration:update'),
  DELETE: Permission('collaboration:delete'),
};