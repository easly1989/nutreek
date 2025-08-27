import { Controller, Get, Post, Put, Delete, Body, Param, Query, Req, UseGuards } from '@nestjs/common';
import { ApiTags, ApiOperation, ApiResponse } from '@nestjs/swagger';
import { CollaborationService, ActivityItem, Comment as CollaborationComment } from './collaboration.service';
import { JwtAuthGuard } from '../auth/jwt-auth.guard';

@ApiTags('collaboration')
@Controller('collaboration')
@UseGuards(JwtAuthGuard)
export class CollaborationController {
  constructor(private readonly collaborationService: CollaborationService) {}

  @Get('activity/:tenantId')
  @ApiOperation({ summary: 'Get activity feed for tenant' })
  @ApiResponse({ status: 200, description: 'Activity feed retrieved successfully' })
  async getActivityFeed(
    @Param('tenantId') tenantId: string,
    @Query('limit') limit?: string,
  ): Promise<ActivityItem[]> {
    const limitNum = limit ? parseInt(limit) : 20;
    return this.collaborationService.getActivityFeed(tenantId, limitNum);
  }

  @Post('comments')
  @ApiOperation({ summary: 'Add comment to entity' })
  @ApiResponse({ status: 201, description: 'Comment added successfully' })
  async addComment(
    @Body() body: { content: string; entityId: string; entityType: 'weekly_plan' | 'recipe' | 'meal' },
    @Req() req,
  ): Promise<CollaborationComment> {
    return this.collaborationService.addComment(
      body.content,
      req.user.id,
      body.entityId,
      body.entityType,
    );
  }

  @Get('comments/:entityId/:entityType')
  @ApiOperation({ summary: 'Get comments for entity' })
  @ApiResponse({ status: 200, description: 'Comments retrieved successfully' })
  async getComments(
    @Param('entityId') entityId: string,
    @Param('entityType') entityType: 'weekly_plan' | 'recipe' | 'meal',
  ): Promise<CollaborationComment[]> {
    return this.collaborationService.getComments(entityId, entityType);
  }

  @Put('comments/:id')
  @ApiOperation({ summary: 'Update comment' })
  @ApiResponse({ status: 200, description: 'Comment updated successfully' })
  async updateComment(
    @Param('id') id: string,
    @Body() body: { content: string },
    @Req() req,
  ): Promise<CollaborationComment> {
    return this.collaborationService.updateComment(id, body.content, req.user.id);
  }

  @Delete('comments/:id')
  @ApiOperation({ summary: 'Delete comment' })
  @ApiResponse({ status: 200, description: 'Comment deleted successfully' })
  async deleteComment(@Param('id') id: string, @Req() req): Promise<void> {
    return this.collaborationService.deleteComment(id, req.user.id);
  }

  @Get('notifications')
  @ApiOperation({ summary: 'Get user notifications' })
  @ApiResponse({ status: 200, description: 'Notifications retrieved successfully' })
  async getNotifications(
    @Req() req,
    @Query('limit') limit?: string,
  ): Promise<any[]> {
    const limitNum = limit ? parseInt(limit) : 10;
    return this.collaborationService.getNotifications(req.user.id, limitNum);
  }

  @Post('notifications/:activityId/read')
  @ApiOperation({ summary: 'Mark notification as read' })
  @ApiResponse({ status: 200, description: 'Notification marked as read' })
  async markNotificationAsRead(
    @Param('activityId') activityId: string,
    @Req() req,
  ): Promise<{ success: boolean }> {
    await this.collaborationService.markNotificationAsRead(activityId, req.user.id);
    return { success: true };
  }
}