import { Controller, Get, Post, Body, Param, Query } from '@nestjs/common';
import { ApiTags, ApiOperation, ApiResponse } from '@nestjs/swagger';
import { IntegrationsService } from './integrations.service';
import {
  WeatherData,
  CalendarEvent,
  SocialMediaPost,
} from './integrations.service';

@ApiTags('integrations')
@Controller('integrations')
export class IntegrationsController {
  constructor(private readonly integrationsService: IntegrationsService) {}

  @Get('nutrition/search')
  @ApiOperation({ summary: 'Search nutrition data from external APIs' })
  @ApiResponse({ status: 200, description: 'Nutrition data retrieved successfully' })
  async searchNutrition(@Query('query') query: string) {
    return this.integrationsService.searchNutritionData(query);
  }

  @Get('weather')
  @ApiOperation({ summary: 'Get weather data for location' })
  @ApiResponse({ status: 200, description: 'Weather data retrieved successfully' })
  async getWeather(@Query('location') location: string) {
    return this.integrationsService.getWeatherData(location);
  }

  @Get('calendar/:userId')
  @ApiOperation({ summary: 'Get calendar events for user' })
  @ApiResponse({ status: 200, description: 'Calendar events retrieved successfully' })
  async getCalendarEvents(
    @Param('userId') userId: string,
    @Query('startDate') startDate: string,
    @Query('endDate') endDate: string,
  ) {
    const start = new Date(startDate);
    const end = new Date(endDate);
    return this.integrationsService.getCalendarEvents(userId, start, end);
  }

  @Get('social-media/:userId')
  @ApiOperation({ summary: 'Get social media posts for user' })
  @ApiResponse({ status: 200, description: 'Social media posts retrieved successfully' })
  async getSocialMediaPosts(
    @Param('userId') userId: string,
    @Query('platforms') platforms: string,
  ) {
    const platformsArray = platforms.split(',');
    return this.integrationsService.getSocialMediaPosts(userId, platformsArray);
  }

  @Get('fitness/:userId')
  @ApiOperation({ summary: 'Get fitness data for user' })
  @ApiResponse({ status: 200, description: 'Fitness data retrieved successfully' })
  async getFitnessData(
    @Param('userId') userId: string,
    @Query('date') date: string,
  ) {
    const dateObj = new Date(date);
    return this.integrationsService.getFitnessData(userId, dateObj);
  }

  @Post('recipes/import')
  @ApiOperation({ summary: 'Import recipe from external URL' })
  @ApiResponse({ status: 200, description: 'Recipe imported successfully' })
  async importRecipe(@Body('url') url: string) {
    return this.integrationsService.importRecipeFromUrl(url);
  }

  @Get('barcode/:barcode')
  @ApiOperation({ summary: 'Get product information by barcode' })
  @ApiResponse({ status: 200, description: 'Product information retrieved successfully' })
  async getProductByBarcode(@Param('barcode') barcode: string) {
    return this.integrationsService.getProductByBarcode(barcode);
  }

  @Post('voice/:userId/:tenantId')
  @ApiOperation({ summary: 'Process voice command' })
  @ApiResponse({ status: 200, description: 'Voice command processed successfully' })
  async processVoiceCommand(
    @Param('userId') userId: string,
    @Param('tenantId') tenantId: string,
    @Body('audioData') audioData: Buffer,
  ) {
    return this.integrationsService.processVoiceCommand(userId, audioData, tenantId);
  }

  @Post('smart-devices/:userId')
  @ApiOperation({ summary: 'Update smart devices with nutrition data' })
  @ApiResponse({ status: 200, description: 'Smart devices updated successfully' })
  async updateSmartDevices(
    @Param('userId') userId: string,
    @Body('nutritionData') nutritionData: any,
  ) {
    await this.integrationsService.updateSmartDevices(userId, nutritionData);
    return { success: true };
  }

  @Post('notifications/:userId')
  @ApiOperation({ summary: 'Send notification to user' })
  @ApiResponse({ status: 200, description: 'Notification sent successfully' })
  async sendNotification(
    @Param('userId') userId: string,
    @Body() body: { type: string; data: any },
  ) {
    await this.integrationsService.sendNotification(userId, body.type, body.data);
    return { success: true };
  }
}