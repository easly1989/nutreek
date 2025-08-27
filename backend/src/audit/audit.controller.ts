import { Controller, Get, Query, Param } from '@nestjs/common';
import { ApiTags, ApiOperation, ApiResponse } from '@nestjs/swagger';
import { AuditService } from './audit.service';

@ApiTags('audit')
@Controller('audit')
export class AuditController {
  constructor(private readonly auditService: AuditService) {}

  @Get('logs')
  @ApiOperation({ summary: 'Get audit logs with filtering' })
  @ApiResponse({ status: 200, description: 'Audit logs retrieved successfully' })
  async getAuditLogs(
    @Query('userId') userId?: string,
    @Query('tenantId') tenantId?: string,
    @Query('resource') resource?: string,
    @Query('action') action?: string,
    @Query('dateFrom') dateFrom?: string,
    @Query('dateTo') dateTo?: string,
    @Query('limit') limit?: string,
    @Query('offset') offset?: string,
  ) {
    const filters = {
      userId,
      tenantId,
      resource,
      action,
      dateFrom: dateFrom ? new Date(dateFrom) : undefined,
      dateTo: dateTo ? new Date(dateTo) : undefined,
      limit: limit ? parseInt(limit) : undefined,
      offset: offset ? parseInt(offset) : undefined,
    };

    return this.auditService.getAuditLogs(filters);
  }

  @Get('users/:userId/activity')
  @ApiOperation({ summary: 'Get user activity logs' })
  @ApiResponse({ status: 200, description: 'User activity retrieved successfully' })
  async getUserActivity(
    @Param('userId') userId: string,
    @Query('limit') limit?: string,
  ) {
    const limitNum = limit ? parseInt(limit) : 20;
    return this.auditService.getUserActivity(userId, limitNum);
  }

  @Get('tenants/:tenantId/activity')
  @ApiOperation({ summary: 'Get tenant activity logs' })
  @ApiResponse({ status: 200, description: 'Tenant activity retrieved successfully' })
  async getTenantActivity(
    @Param('tenantId') tenantId: string,
    @Query('limit') limit?: string,
  ) {
    const limitNum = limit ? parseInt(limit) : 50;
    return this.auditService.getTenantActivity(tenantId, limitNum);
  }

  @Get('security-events')
  @ApiOperation({ summary: 'Get security events' })
  @ApiResponse({ status: 200, description: 'Security events retrieved successfully' })
  async getSecurityEvents(
    @Query('type') type?: string,
    @Query('severity') severity?: string,
    @Query('resolved') resolved?: string,
    @Query('dateFrom') dateFrom?: string,
    @Query('dateTo') dateTo?: string,
    @Query('limit') limit?: string,
  ) {
    const filters = {
      type,
      severity,
      resolved: resolved ? resolved === 'true' : undefined,
      dateFrom: dateFrom ? new Date(dateFrom) : undefined,
      dateTo: dateTo ? new Date(dateTo) : undefined,
      limit: limit ? parseInt(limit) : undefined,
    };

    return this.auditService.getSecurityEvents(filters);
  }

  @Get('reports/user-activity')
  @ApiOperation({ summary: 'Get user activity report' })
  @ApiResponse({ status: 200, description: 'User activity report generated' })
  async getUserActivityReport(
    @Query('dateFrom') dateFrom: string,
    @Query('dateTo') dateTo: string,
  ) {
    const startDate = new Date(dateFrom);
    const endDate = new Date(dateTo);

    const logs = await this.auditService.getAuditLogs({
      dateFrom: startDate,
      dateTo: endDate,
    });

    // Group by user and count activities
    const userActivity = logs.reduce((acc, log) => {
      const userId = log.userId || 'anonymous';
      if (!acc[userId]) {
        acc[userId] = {
          userId,
          userName: log.user?.name || 'Unknown',
          userEmail: log.user?.email || 'Unknown',
          totalActivities: 0,
          activitiesByType: {},
          lastActivity: log.timestamp,
        };
      }

      acc[userId].totalActivities++;
      acc[userId].activitiesByType[log.action] = (acc[userId].activitiesByType[log.action] || 0) + 1;

      if (log.timestamp > acc[userId].lastActivity) {
        acc[userId].lastActivity = log.timestamp;
      }

      return acc;
    }, {});

    return {
      period: { startDate, endDate },
      summary: {
        totalLogs: logs.length,
        uniqueUsers: Object.keys(userActivity).length,
      },
      userActivity: Object.values(userActivity),
    };
  }

  @Get('reports/security-summary')
  @ApiOperation({ summary: 'Get security events summary' })
  @ApiResponse({ status: 200, description: 'Security summary generated' })
  async getSecuritySummary(
    @Query('dateFrom') dateFrom: string,
    @Query('dateTo') dateTo: string,
  ) {
    const startDate = new Date(dateFrom);
    const endDate = new Date(dateTo);

    const events = await this.auditService.getSecurityEvents({
      dateFrom: startDate,
      dateTo: endDate,
    });

    const summary = events.reduce((acc, event) => {
      acc.total++;
      acc.byType[event.type] = (acc.byType[event.type] || 0) + 1;
      acc.bySeverity[event.severity] = (acc.bySeverity[event.severity] || 0) + 1;
      acc.resolved += event.resolved ? 1 : 0;
      return acc;
    }, {
      total: 0,
      resolved: 0,
      byType: {},
      bySeverity: {},
    });

    return {
      period: { startDate, endDate },
      summary,
      recentEvents: events.slice(0, 10),
    };
  }

  @Get('reports/compliance')
  @ApiOperation({ summary: 'Get compliance report' })
  @ApiResponse({ status: 200, description: 'Compliance report generated' })
  async getComplianceReport(
    @Query('dateFrom') dateFrom: string,
    @Query('dateTo') dateTo: string,
  ) {
    const startDate = new Date(dateFrom);
    const endDate = new Date(dateTo);

    const [logs, securityEvents] = await Promise.all([
      this.auditService.getAuditLogs({
        dateFrom: startDate,
        dateTo: endDate,
      }),
      this.auditService.getSecurityEvents({
        dateFrom: startDate,
        dateTo: endDate,
      }),
    ]);

    // Analyze data for compliance metrics
    const compliance = {
      period: { startDate, endDate },
      metrics: {
        totalActivities: logs.length,
        securityEvents: securityEvents.length,
        failedSecurityEvents: securityEvents.filter(e => e.severity === 'HIGH' || e.severity === 'CRITICAL').length,
        uniqueUsers: new Set(logs.map(log => log.userId).filter(Boolean)).size,
        dataAccessActivities: logs.filter(log => log.resource === 'user' && log.action === 'READ').length,
        dataModificationActivities: logs.filter(log => ['CREATE', 'UPDATE', 'DELETE'].includes(log.action)).length,
      },
      riskIndicators: {
        highSeverityEvents: securityEvents.filter(e => e.severity === 'HIGH').length,
        criticalSeverityEvents: securityEvents.filter(e => e.severity === 'CRITICAL').length,
        unresolvedSecurityEvents: securityEvents.filter(e => !e.resolved).length,
      },
      recommendations: [],
    };

    // Generate recommendations
    if (compliance.riskIndicators.criticalSeverityEvents > 0) {
      compliance.recommendations.push('Immediate attention required for critical security events');
    }
    if (compliance.riskIndicators.unresolvedSecurityEvents > 10) {
      compliance.recommendations.push('Review and resolve outstanding security events');
    }
    if (compliance.metrics.dataAccessActivities > compliance.metrics.totalActivities * 0.5) {
      compliance.recommendations.push('High volume of data access activities detected');
    }

    return compliance;
  }
}