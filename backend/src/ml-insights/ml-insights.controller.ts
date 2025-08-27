import { Controller, Get, Post, Body, Param, Query } from '@nestjs/common';
import { ApiTags, ApiOperation, ApiResponse } from '@nestjs/swagger';
import { MlInsightsService, NutritionPattern, MealRecommendation, WeeklyPlanPrediction, UserPreferences } from './ml-insights.service';

@ApiTags('ml-insights')
@Controller('ml-insights')
export class MlInsightsController {
  constructor(private readonly mlInsightsService: MlInsightsService) {}

  @Get('nutrition-patterns/:tenantId')
  @ApiOperation({ summary: 'Analyze nutrition patterns for tenant' })
  @ApiResponse({ status: 200, description: 'Nutrition patterns analyzed successfully' })
  async getNutritionPatterns(@Param('tenantId') tenantId: string, @Query('userId') userId?: string) {
    return this.mlInsightsService.analyzeNutritionPatterns(tenantId, userId);
  }

  @Post('meal-recommendations/:tenantId')
  @ApiOperation({ summary: 'Generate personalized meal recommendations' })
  @ApiResponse({ status: 200, description: 'Meal recommendations generated successfully' })
  async getMealRecommendations(
    @Param('tenantId') tenantId: string,
    @Body() body: {
      dietaryRestrictions: string[];
      allergies: string[];
      preferredCuisines: string[];
      dislikedIngredients: string[];
      nutritionalGoals?: {
        targetCalories?: number;
        targetProtein?: number;
        targetCarbs?: number;
        targetFat?: number;
      };
    },
    @Query('currentWeekData') currentWeekData?: string,
  ) {
    const currentWeek = currentWeekData ? JSON.parse(currentWeekData) : undefined;
    return this.mlInsightsService.generateMealRecommendations(tenantId, body, currentWeek);
  }

  @Post('weekly-prediction/:tenantId')
  @ApiOperation({ summary: 'Predict weekly nutrition based on plan' })
  @ApiResponse({ status: 200, description: 'Weekly nutrition predicted successfully' })
  async predictWeeklyNutrition(@Param('tenantId') tenantId: string, @Body() planData: any) {
    return this.mlInsightsService.predictWeeklyNutrition(tenantId, planData);
  }

  @Get('recipe-performance/:tenantId')
  @ApiOperation({ summary: 'Analyze recipe performance and trends' })
  @ApiResponse({ status: 200, description: 'Recipe performance analyzed successfully' })
  async getRecipePerformance(@Param('tenantId') tenantId: string) {
    return this.mlInsightsService.analyzeRecipePerformance(tenantId);
  }

  @Get('personalized-insights/:userId/:tenantId')
  @ApiOperation({ summary: 'Generate personalized nutrition insights' })
  @ApiResponse({ status: 200, description: 'Personalized insights generated successfully' })
  async getPersonalizedInsights(@Param('userId') userId: string, @Param('tenantId') tenantId: string) {
    return this.mlInsightsService.generatePersonalizedInsights(userId, tenantId);
  }
}