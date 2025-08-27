import { Injectable } from '@nestjs/common';
import { PrismaService } from '../prisma.service';

export interface AuditLogData {
  userId?: string;
  tenantId?: string;
  action: string;
  resource: string;
  resourceId?: string;
  details?: any;
  ipAddress?: string;
  userAgent?: string;
  success?: boolean;
  errorMessage?: string;
}

@Injectable()
export class AuditService {
  constructor(private prisma: PrismaService) {}

  async logActivity(data: AuditLogData): Promise<void> {
    try {
      await this.prisma.auditLog.create({
        data: {
          userId: data.userId,
          tenantId: data.tenantId,
          action: data.action,
          resource: data.resource,
          resourceId: data.resourceId,
          details: data.details || {},
          ipAddress: data.ipAddress,
          userAgent: data.userAgent,
          success: data.success !== false,
          errorMessage: data.errorMessage,
        },
      });
    } catch (error) {
      console.error('Failed to log audit activity:', error);
      // Don't throw error to avoid disrupting main business logic
    }
  }

  async getAuditLogs(filters: {
    userId?: string;
    tenantId?: string;
    resource?: string;
    action?: string;
    dateFrom?: Date;
    dateTo?: Date;
    limit?: number;
    offset?: number;
  }) {
    const where: any = {};

    if (filters.userId) where.userId = filters.userId;
    if (filters.tenantId) where.tenantId = filters.tenantId;
    if (filters.resource) where.resource = filters.resource;
    if (filters.action) where.action = filters.action;

    if (filters.dateFrom || filters.dateTo) {
      where.timestamp = {};
      if (filters.dateFrom) where.timestamp.gte = filters.dateFrom;
      if (filters.dateTo) where.timestamp.lte = filters.dateTo;
    }

    return this.prisma.auditLog.findMany({
      where,
      include: {
        user: {
          select: { id: true, name: true, email: true }
        },
        tenant: {
          select: { id: true, name: true }
        }
      },
      orderBy: { timestamp: 'desc' },
      take: filters.limit || 50,
      skip: filters.offset || 0,
    });
  }

  async getUserActivity(userId: string, limit = 20) {
    return this.prisma.auditLog.findMany({
      where: { userId },
      include: {
        tenant: {
          select: { id: true, name: true }
        }
      },
      orderBy: { timestamp: 'desc' },
      take: limit,
    });
  }

  async getTenantActivity(tenantId: string, limit = 50) {
    return this.prisma.auditLog.findMany({
      where: { tenantId },
      include: {
        user: {
          select: { id: true, name: true, email: true }
        }
      },
      orderBy: { timestamp: 'desc' },
      take: limit,
    });
  }

  async getSecurityEvents(filters: {
    type?: string;
    severity?: string;
    resolved?: boolean;
    dateFrom?: Date;
    dateTo?: Date;
    limit?: number;
  }) {
    const where: any = {};

    if (filters.type) where.type = filters.type;
    if (filters.severity) where.severity = filters.severity;
    if (filters.resolved !== undefined) where.resolved = filters.resolved;

    if (filters.dateFrom || filters.dateTo) {
      where.timestamp = {};
      if (filters.dateFrom) where.timestamp.gte = filters.dateFrom;
      if (filters.dateTo) where.timestamp.lte = filters.dateTo;
    }

    return this.prisma.securityEvent.findMany({
      where,
      include: {
        user: {
          select: { id: true, name: true, email: true }
        },
        tenant: {
          select: { id: true, name: true }
        }
      },
      orderBy: { timestamp: 'desc' },
      take: filters.limit || 50,
    });
  }

  async logSecurityEvent(data: {
    type: string;
    severity: string;
    userId?: string;
    tenantId?: string;
    details?: any;
    ipAddress?: string;
    userAgent?: string;
  }): Promise<void> {
    try {
      await this.prisma.securityEvent.create({
        data: {
          type: data.type,
          severity: data.severity,
          userId: data.userId,
          tenantId: data.tenantId,
          details: data.details || {},
          ipAddress: data.ipAddress,
          userAgent: data.userAgent,
        },
      });
    } catch (error) {
      console.error('Failed to log security event:', error);
    }
  }

  async resolveSecurityEvent(eventId: string): Promise<void> {
    await this.prisma.securityEvent.update({
      where: { id: eventId },
      data: { resolved: true }
    });
  }

  // Convenience methods for common audit actions
  async logUserAction(userId: string, action: string, resource: string, resourceId?: string, details?: any, success = true, errorMessage?: string): Promise<void> {
    await this.logActivity({
      userId,
      action,
      resource,
      resourceId,
      details,
      success,
      errorMessage,
    });
  }

  async logTenantAction(userId: string, tenantId: string, action: string, resource: string, resourceId?: string, details?: any, success = true, errorMessage?: string): Promise<void> {
    await this.logActivity({
      userId,
      tenantId,
      action,
      resource,
      resourceId,
      details,
      success,
      errorMessage,
    });
  }

  async logFailedLogin(email: string, ipAddress?: string, userAgent?: string): Promise<void> {
    await this.logSecurityEvent({
      type: 'FAILED_LOGIN',
      severity: 'LOW',
      details: { email },
      ipAddress,
      userAgent,
    });
  }

  async logSuspiciousActivity(userId: string, activity: string, ipAddress?: string, userAgent?: string): Promise<void> {
    await this.logSecurityEvent({
      type: 'SUSPICIOUS_ACTIVITY',
      severity: 'MEDIUM',
      userId,
      details: { activity },
      ipAddress,
      userAgent,
    });
  }

  async logRateLimitExceeded(identifier: string, ipAddress?: string): Promise<void> {
    await this.logSecurityEvent({
      type: 'RATE_LIMIT_EXCEEDED',
      severity: 'MEDIUM',
      details: { identifier },
      ipAddress,
    });
  }

  async logDataExport(userId: string, exportType: string, tenantId?: string): Promise<void> {
    await this.logActivity({
      userId,
      tenantId,
      action: 'EXPORT',
      resource: 'data',
      details: { exportType },
    });
  }

  async logPermissionChange(userId: string, tenantId: string, action: string, targetUserId: string, roleName: string): Promise<void> {
    await this.logActivity({
      userId,
      tenantId,
      action,
      resource: 'permission',
      resourceId: targetUserId,
      details: { roleName },
    });
  }
}