import { Injectable, NotFoundException, BadRequestException } from '@nestjs/common';
import { PrismaService } from '../prisma.service';
import { AuditService } from '../audit/audit.service';

@Injectable()
export class ComplianceService {
  constructor(
    private prisma: PrismaService,
    private auditService: AuditService,
  ) {}

  async requestDataExport(userId: string, tenantId?: string): Promise<string> {
    // Create compliance record
    const record = await this.prisma.complianceRecord.create({
      data: {
        userId,
        type: 'DATA_EXPORT',
        status: 'PENDING',
        details: {
          tenantId,
          exportType: tenantId ? 'TENANT_DATA' : 'ALL_USER_DATA',
        },
        expiresAt: new Date(Date.now() + 30 * 24 * 60 * 60 * 1000), // 30 days
      },
    });

    // Log the request
    await this.auditService.logDataExport(userId, tenantId ? 'TENANT_DATA' : 'ALL_USER_DATA', tenantId);

    // Start background export process
    this.processDataExport(record.id);

    return record.id;
  }

  async requestDataDeletion(userId: string, tenantId?: string): Promise<string> {
    const record = await this.prisma.complianceRecord.create({
      data: {
        userId,
        type: 'DATA_DELETION',
        status: 'PENDING',
        details: {
          tenantId,
          deletionType: tenantId ? 'TENANT_DATA' : 'ALL_USER_DATA',
        },
        expiresAt: new Date(Date.now() + 30 * 24 * 60 * 60 * 1000), // 30 days
      },
    });

    await this.auditService.logUserAction(
      userId,
      'DELETE',
      'user_data',
      undefined,
      { tenantId, complianceRecordId: record.id }
    );

    // Start background deletion process
    this.processDataDeletion(record.id);

    return record.id;
  }

  async getComplianceRecords(userId: string): Promise<any[]> {
    return this.prisma.complianceRecord.findMany({
      where: { userId },
      orderBy: { requestedAt: 'desc' },
    });
  }

  async getComplianceRecord(recordId: string): Promise<any> {
    const record = await this.prisma.complianceRecord.findUnique({
      where: { id: recordId },
      include: {
        user: {
          select: { id: true, name: true, email: true }
        }
      }
    });

    if (!record) {
      throw new NotFoundException('Compliance record not found');
    }

    return record;
  }

  async updateComplianceRecord(recordId: string, updates: { status?: string; details?: any }): Promise<void> {
    await this.prisma.complianceRecord.update({
      where: { id: recordId },
      data: {
        ...updates,
        completedAt: updates.status === 'COMPLETED' ? new Date() : undefined,
      },
    });
  }

  private async processDataExport(recordId: string): Promise<void> {
    try {
      const record = await this.getComplianceRecord(recordId);
      const { userId, details } = record;
      const { tenantId, exportType } = details;

      // Update status to processing
      await this.updateComplianceRecord(recordId, { status: 'IN_PROGRESS' });

      // Collect all user data
      const userData = await this.collectUserData(userId, tenantId);

      // Create export file (in a real implementation, this would be a secure file storage)
      const exportData = {
        userId,
        tenantId,
        exportType,
        exportedAt: new Date(),
        data: userData,
      };

      // Create data export record
      const dataExport = await this.prisma.dataExport.create({
        data: {
          userId,
          tenantId,
          type: exportType,
          status: 'COMPLETED',
          fileUrl: `exports/${recordId}.json`, // Placeholder URL
          completedAt: new Date(),
          expiresAt: new Date(Date.now() + 30 * 24 * 60 * 60 * 1000), // 30 days
        },
      });

      // Update compliance record
      await this.updateComplianceRecord(recordId, {
        status: 'COMPLETED',
        details: {
          ...details,
          exportId: dataExport.id,
          dataSummary: this.summarizeExportedData(userData),
        },
      });

    } catch (error) {
      console.error('Data export failed:', error);
      await this.updateComplianceRecord(recordId, {
        status: 'FAILED',
        details: { error: error.message },
      });
    }
  }

  private async processDataDeletion(recordId: string): Promise<void> {
    try {
      const record = await this.getComplianceRecord(recordId);
      const { userId, details } = record;
      const { tenantId, deletionType } = details;

      // Update status to processing
      await this.updateComplianceRecord(recordId, { status: 'IN_PROGRESS' });

      if (deletionType === 'TENANT_DATA' && tenantId) {
        // Delete tenant-specific data
        await this.deleteTenantData(userId, tenantId);
      } else {
        // Delete all user data
        await this.deleteAllUserData(userId);
      }

      // Update compliance record
      await this.updateComplianceRecord(recordId, {
        status: 'COMPLETED',
        details: {
          ...details,
          deletedAt: new Date(),
        },
      });

    } catch (error) {
      console.error('Data deletion failed:', error);
      await this.updateComplianceRecord(recordId, {
        status: 'FAILED',
        details: { error: error.message },
      });
    }
  }

  private async collectUserData(userId: string, tenantId?: string): Promise<any> {
    const data = {
      user: null,
      memberships: [],
      weeklyPlans: [],
      recipes: [],
      ingredients: [],
      activities: [],
      comments: [],
      auditLogs: [],
      dataExports: [],
      complianceRecords: [],
    };

    // User profile
    data.user = await this.prisma.user.findUnique({
      where: { id: userId },
      select: {
        id: true,
        email: true,
        name: true,
        createdAt: true,
        updatedAt: true,
      },
    });

    // Memberships
    data.memberships = await this.prisma.membership.findMany({
      where: { userId },
      include: {
        tenant: {
          select: { id: true, name: true }
        },
        role: {
          select: { id: true, name: true, description: true }
        }
      },
    });

    // Tenant-specific data
    if (tenantId) {
      data.weeklyPlans = await this.prisma.weeklyPlan.findMany({
        where: { tenantId },
        include: {
          days: {
            include: {
              meals: {
                include: {
                  recipes: {
                    include: {
                      ingredients: true,
                    },
                  },
                },
              },
            },
          },
          shoppingList: true,
        },
      });

      data.activities = await this.prisma.activity.findMany({
        where: { tenantId, userId },
      });

      data.comments = await this.prisma.comment.findMany({
        where: { userId },
      });
    }

    // User-specific data
    data.auditLogs = await this.prisma.auditLog.findMany({
      where: { userId },
    });

    data.dataExports = await this.prisma.dataExport.findMany({
      where: { userId },
    });

    data.complianceRecords = await this.prisma.complianceRecord.findMany({
      where: { userId },
    });

    return data;
  }

  private async deleteTenantData(userId: string, tenantId: string): Promise<void> {
    // Remove user from tenant
    await this.prisma.membership.deleteMany({
      where: {
        userId,
        tenantId,
      },
    });

    // Delete user's activities in this tenant
    await this.prisma.activity.deleteMany({
      where: {
        userId,
        tenantId,
      },
    });

    // Delete user's comments
    await this.prisma.comment.deleteMany({
      where: { userId },
    });
  }

  private async deleteAllUserData(userId: string): Promise<void> {
    // Delete all memberships
    await this.prisma.membership.deleteMany({
      where: { userId },
    });

    // Delete all activities
    await this.prisma.activity.deleteMany({
      where: { userId },
    });

    // Delete all comments
    await this.prisma.comment.deleteMany({
      where: { userId },
    });

    // Delete audit logs
    await this.prisma.auditLog.deleteMany({
      where: { userId },
    });

    // Delete data exports
    await this.prisma.dataExport.deleteMany({
      where: { userId },
    });

    // Delete compliance records
    await this.prisma.complianceRecord.deleteMany({
      where: { userId },
    });

    // Delete security events
    await this.prisma.securityEvent.deleteMany({
      where: { userId },
    });

    // Finally delete the user
    await this.prisma.user.delete({
      where: { id: userId },
    });
  }

  private summarizeExportedData(data: any): any {
    return {
      user: !!data.user,
      memberships: data.memberships.length,
      weeklyPlans: data.weeklyPlans.length,
      activities: data.activities.length,
      comments: data.comments.length,
      auditLogs: data.auditLogs.length,
      dataExports: data.dataExports.length,
      complianceRecords: data.complianceRecords.length,
    };
  }

  async getDataRetentionPolicy(): Promise<any> {
    return {
      userData: '2 years after account deletion',
      auditLogs: '7 years',
      securityEvents: '2 years',
      complianceRecords: '10 years',
      dataExports: '30 days after creation',
    };
  }

  async anonymizeUserData(userId: string): Promise<void> {
    // Create compliance record
    const record = await this.prisma.complianceRecord.create({
      data: {
        userId,
        type: 'DATA_ANONYMIZATION',
        status: 'COMPLETED',
        details: {
          anonymizedAt: new Date(),
        },
        completedAt: new Date(),
      },
    });

    // In a real implementation, this would anonymize data while preserving functionality
    await this.prisma.user.update({
      where: { id: userId },
      data: {
        name: 'Anonymous User',
        email: `anonymous-${userId}@deleted.local`,
      },
    });

    await this.auditService.logUserAction(
      userId,
      'ANONYMIZE',
      'user_data',
      undefined,
      { complianceRecordId: record.id }
    );
  }
}