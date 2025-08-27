import { Controller, Get, Post, Param, Patch, Body } from '@nestjs/common';
import { ApiTags, ApiOperation, ApiResponse } from '@nestjs/swagger';
import { ShoppingListsService, ShoppingList } from './shopping-lists.service';

@ApiTags('shopping-lists')
@Controller('shopping-lists')
export class ShoppingListsController {
  constructor(private readonly shoppingListsService: ShoppingListsService) {}

  @Post('generate/:weeklyPlanId')
  @ApiOperation({ summary: 'Generate shopping list from weekly plan' })
  @ApiResponse({ status: 201, description: 'Shopping list generated successfully' })
  async generateFromWeeklyPlan(@Param('weeklyPlanId') weeklyPlanId: string) {
    return this.shoppingListsService.generateFromWeeklyPlan(weeklyPlanId);
  }

  @Get('weekly-plan/:weeklyPlanId')
  @ApiOperation({ summary: 'Get shopping list for weekly plan' })
  @ApiResponse({ status: 200, description: 'Shopping list retrieved successfully' })
  async findByWeeklyPlan(@Param('weeklyPlanId') weeklyPlanId: string) {
    return this.shoppingListsService.findByWeeklyPlan(weeklyPlanId);
  }

  @Get('tenant/:tenantId')
  @ApiOperation({ summary: 'Get all shopping lists for tenant' })
  @ApiResponse({ status: 200, description: 'Shopping lists retrieved successfully' })
  async getByTenant(@Param('tenantId') tenantId: string) {
    return this.shoppingListsService.getByTenant(tenantId);
  }

  @Patch('weekly-plan/:weeklyPlanId/item/:ingredientId')
  @ApiOperation({ summary: 'Update shopping list item status' })
  @ApiResponse({ status: 200, description: 'Item status updated successfully' })
  async updateItemStatus(
    @Param('weeklyPlanId') weeklyPlanId: string,
    @Param('ingredientId') ingredientId: string,
    @Body('purchased') purchased: boolean,
  ) {
    await this.shoppingListsService.updateItemStatus(weeklyPlanId, ingredientId, purchased);
    return { success: true };
  }
}