import { Controller, Get, Post, Body, Put, Param, Delete } from '@nestjs/common';
import { ApiTags, ApiOperation, ApiResponse } from '@nestjs/swagger';
import { PlansService, CreateWeeklyPlanDto, UpdateWeeklyPlanDto } from './plans.service';

@ApiTags('plans')
@Controller('tenants/:tenantId/plans')
export class PlansController {
  constructor(private readonly plansService: PlansService) {}

  @Post()
  @ApiOperation({ summary: 'Create a new weekly plan' })
  @ApiResponse({ status: 201, description: 'Weekly plan created successfully' })
  create(@Param('tenantId') tenantId: string, @Body() createWeeklyPlanDto: CreateWeeklyPlanDto) {
    return this.plansService.create(tenantId, createWeeklyPlanDto);
  }

  @Get()
  @ApiOperation({ summary: 'Get all weekly plans for tenant' })
  @ApiResponse({ status: 200, description: 'List of weekly plans' })
  findAll(@Param('tenantId') tenantId: string) {
    return this.plansService.findAll(tenantId);
  }

  @Get(':planId')
  @ApiOperation({ summary: 'Get weekly plan by ID' })
  @ApiResponse({ status: 200, description: 'Weekly plan details' })
  findOne(@Param('planId') planId: string) {
    return this.plansService.findOne(planId);
  }

  @Put(':planId')
  @ApiOperation({ summary: 'Update weekly plan' })
  @ApiResponse({ status: 200, description: 'Weekly plan updated successfully' })
  update(@Param('planId') planId: string, @Body() updateWeeklyPlanDto: UpdateWeeklyPlanDto) {
    return this.plansService.update(planId, updateWeeklyPlanDto);
  }

  @Delete(':planId')
  @ApiOperation({ summary: 'Delete weekly plan' })
  @ApiResponse({ status: 200, description: 'Weekly plan deleted successfully' })
  remove(@Param('planId') planId: string) {
    return this.plansService.remove(planId);
  }

  @Get('ui-metadata/config')
  @ApiOperation({ summary: 'Get UI metadata configuration' })
  @ApiResponse({ status: 200, description: 'UI metadata configuration' })
  getUIMetadata() {
    return this.plansService.getUIMetadata();
  }

  @Get('ui-metadata/meal-types')
  @ApiOperation({ summary: 'Get meal type configurations with UI metadata' })
  @ApiResponse({ status: 200, description: 'Meal type configurations' })
  getMealTypeConfigs() {
    return this.plansService.getMealTypeConfigs();
  }
}