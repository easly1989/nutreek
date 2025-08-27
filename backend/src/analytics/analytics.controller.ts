import { Controller, Get, Query, Param } from '@nestjs/common';
import { ApiTags, ApiOperation, ApiResponse } from '@nestjs/swagger';
import { AnalyticsService, NutritionSummary, RecipeStats, HouseholdStats } from './analytics.service';

@ApiTags('analytics')
@Controller('analytics')
export class AnalyticsController {
  constructor(private readonly analyticsService: AnalyticsService) {}

  @Get('nutrition/:tenantId')
  @ApiOperation({ summary: 'Get nutrition summary for tenant' })
  @ApiResponse({ status: 200, description: 'Nutrition summary retrieved successfully' })
  async getNutritionSummary(
    @Param('tenantId') tenantId: string,
    @Query('startDate') startDate?: string,
    @Query('endDate') endDate?: string,
  ) {
    const start = startDate ? new Date(startDate) : undefined;
    const end = endDate ? new Date(endDate) : undefined;
    return this.analyticsService.getNutritionSummary(tenantId, start, end);
  }

  @Get('recipes/:tenantId')
  @ApiOperation({ summary: 'Get recipe statistics for tenant' })
  @ApiResponse({ status: 200, description: 'Recipe statistics retrieved successfully' })
  async getRecipeStats(@Param('tenantId') tenantId: string) {
    return this.analyticsService.getRecipeStats(tenantId);
  }

  @Get('household/:tenantId')
  @ApiOperation({ summary: 'Get household statistics' })
  @ApiResponse({ status: 200, description: 'Household statistics retrieved successfully' })
  async getHouseholdStats(@Param('tenantId') tenantId: string) {
    return this.analyticsService.getHouseholdStats(tenantId);
  }

  @Get('trends/:tenantId')
  @ApiOperation({ summary: 'Get weekly nutrition trends' })
  @ApiResponse({ status: 200, description: 'Weekly trends retrieved successfully' })
  async getWeeklyTrends(
    @Param('tenantId') tenantId: string,
    @Query('weeks') weeks?: string,
  ) {
    const weeksNum = weeks ? parseInt(weeks) : 4;
    return this.analyticsService.getWeeklyTrends(tenantId, weeksNum);
  }
}