import { Injectable } from '@nestjs/common';
import { PrismaService } from '../prisma.service';

export interface ActivityItem {
  id: string;
  type: 'recipe_created' | 'meal_added' | 'plan_created' | 'member_joined' | 'comment_added';
  title: string;
  description: string;
  userId: string;
  userName: string;
  tenantId: string;
  entityId?: string;
  entityType?: string;
  metadata?: any;
  createdAt: Date;
}

export interface Comment {
  id: string;
  content: string;
  userId: string;
  userName: string;
  entityId: string;
  entityType: 'weekly_plan' | 'recipe' | 'meal';
  createdAt: Date;
  updatedAt: Date;
}

@Injectable()
export class CollaborationService {
  constructor(private prisma: PrismaService) {}

  async createActivity(
    type: ActivityItem['type'],
    title: string,
    description: string,
    userId: string,
    tenantId: string,
    entityId?: string,
    entityType?: string,
    metadata?: any,
  ): Promise<ActivityItem> {
    const user = await this.prisma.user.findUnique({
      where: { id: userId },
      select: { name: true, email: true },
    });

    const activity = await this.prisma.activity.create({
      data: {
        type,
        title,
        description,
        userId,
        tenantId,
        entityId,
        entityType,
        metadata,
      },
    });

    return {
      ...activity,
      userName: user?.name || user?.email || 'Unknown User',
    } as ActivityItem;
  }

  async getActivityFeed(tenantId: string, limit: number = 20): Promise<ActivityItem[]> {
    const activities = await this.prisma.activity.findMany({
      where: { tenantId },
      include: {
        user: {
          select: { name: true, email: true },
        },
      },
      orderBy: { createdAt: 'desc' },
      take: limit,
    });

    return activities.map(activity => ({
      ...activity,
      userName: activity.user?.name || activity.user?.email || 'Unknown User',
    })) as ActivityItem[];
  }

  async addComment(
    content: string,
    userId: string,
    entityId: string,
    entityType: Comment['entityType'],
  ): Promise<Comment> {
    const user = await this.prisma.user.findUnique({
      where: { id: userId },
      select: { name: true, email: true },
    });

    const comment = await this.prisma.comment.create({
      data: {
        content,
        userId,
        entityId,
        entityType,
      },
    });

    // Create activity for the comment
    const entityDisplayName = await this.getEntityDisplayName(entityId, entityType);
    await this.createActivity(
      'comment_added',
      'Comment Added',
      `Commented on ${entityDisplayName}`,
      userId,
      await this.getTenantIdFromEntity(entityId, entityType),
      entityId,
      entityType,
    );

    return {
      ...comment,
      userName: user?.name || user?.email || 'Unknown User',
    } as Comment;
  }

  async getComments(entityId: string, entityType: Comment['entityType']): Promise<Comment[]> {
    const comments = await this.prisma.comment.findMany({
      where: {
        entityId,
        entityType,
      },
      include: {
        user: {
          select: { name: true, email: true },
        },
      },
      orderBy: { createdAt: 'asc' },
    });

    return comments.map(comment => ({
      ...comment,
      userName: comment.user?.name || comment.user?.email || 'Unknown User',
    })) as Comment[];
  }

  async updateComment(id: string, content: string, userId: string): Promise<Comment> {
    const comment = await this.prisma.comment.update({
      where: { id },
      data: { content, updatedAt: new Date() },
      include: {
        user: {
          select: { name: true, email: true },
        },
      },
    });

    return {
      ...comment,
      userName: comment.user?.name || comment.user?.email || 'Unknown User',
    } as Comment;
  }

  async deleteComment(id: string, userId: string): Promise<void> {
    await this.prisma.comment.delete({
      where: { id },
    });
  }

  async getNotifications(userId: string, limit: number = 10): Promise<any[]> {
    // Get activities from all tenants the user belongs to
    const memberships = await this.prisma.membership.findMany({
      where: { userId },
      select: { tenantId: true },
    });

    const tenantIds = memberships.map(m => m.tenantId);

    const activities = await this.prisma.activity.findMany({
      where: {
        tenantId: { in: tenantIds },
        userId: { not: userId }, // Exclude user's own activities
      },
      include: {
        user: {
          select: { name: true, email: true },
        },
        tenant: {
          select: { name: true },
        },
      },
      orderBy: { createdAt: 'desc' },
      take: limit,
    });

    return activities.map(activity => ({
      ...activity,
      userName: activity.user?.name || activity.user?.email || 'Unknown User',
      tenantName: activity.tenant?.name || 'Unknown Household',
    }));
  }

  async markNotificationAsRead(activityId: string, userId: string): Promise<void> {
    // In a real implementation, you'd have a notification_reads table
    // For now, we'll just acknowledge that this could be implemented
    console.log(`Marking notification ${activityId} as read for user ${userId}`);
  }

  private async getEntityDisplayName(entityId: string, entityType: string): Promise<string> {
    switch (entityType) {
      case 'weekly_plan':
        const plan = await this.prisma.weeklyPlan.findUnique({
          where: { id: entityId },
          select: { startDate: true },
        });
        return plan ? `plan for ${plan.startDate.toDateString()}` : 'weekly plan';

      case 'recipe':
        const recipe = await this.prisma.recipe.findUnique({
          where: { id: entityId },
          select: { name: true },
        });
        return recipe ? recipe.name : 'recipe';

      case 'meal':
        const meal = await this.prisma.meal.findUnique({
          where: { id: entityId },
          select: { type: true },
        });
        return meal ? `${meal.type} meal` : 'meal';

      default:
        return entityType;
    }
  }

  private async getTenantIdFromEntity(entityId: string, entityType: string): Promise<string> {
    switch (entityType) {
      case 'weekly_plan':
        const plan = await this.prisma.weeklyPlan.findUnique({
          where: { id: entityId },
          select: { tenantId: true },
        });
        return plan?.tenantId || '';

      case 'recipe':
        const recipe = await this.prisma.recipe.findUnique({
          where: { id: entityId },
          include: {
            meal: {
              include: {
                day: {
                  include: {
                    weeklyPlan: {
                      select: { tenantId: true },
                    },
                  },
                },
              },
            },
          },
        });
        return recipe?.meal?.day?.weeklyPlan?.tenantId || '';

      case 'meal':
        const meal = await this.prisma.meal.findUnique({
          where: { id: entityId },
          include: {
            day: {
              include: {
                weeklyPlan: {
                  select: { tenantId: true },
                },
              },
            },
          },
        });
        return meal?.day?.weeklyPlan?.tenantId || '';

      default:
        return '';
    }
  }
}