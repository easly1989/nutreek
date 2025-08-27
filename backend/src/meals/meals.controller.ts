import { Controller, Get, Post, Body, Patch, Param, Delete } from '@nestjs/common';
import { ApiTags, ApiOperation, ApiResponse } from '@nestjs/swagger';
import { MealsService, CreateMealDto, UpdateMealDto, AddRecipeDto } from './meals.service';

@ApiTags('meals')
@Controller('days/:dayId/meals')
export class MealsController {
  constructor(private readonly mealsService: MealsService) {}

  @Post()
  @ApiOperation({ summary: 'Create a new meal' })
  @ApiResponse({ status: 201, description: 'Meal created successfully' })
  create(@Param('dayId') dayId: string, @Body() createMealDto: Omit<CreateMealDto, 'dayId'>) {
    return this.mealsService.create(dayId, createMealDto);
  }

  @Get()
  @ApiOperation({ summary: 'Get all meals for a day' })
  @ApiResponse({ status: 200, description: 'List of meals' })
  findAll(@Param('dayId') dayId: string) {
    return this.mealsService.findAll(dayId);
  }

  @Get(':id')
  @ApiOperation({ summary: 'Get meal by ID' })
  @ApiResponse({ status: 200, description: 'Meal details' })
  findOne(@Param('id') id: string) {
    return this.mealsService.findOne(id);
  }

  @Patch(':id')
  @ApiOperation({ summary: 'Update meal' })
  @ApiResponse({ status: 200, description: 'Meal updated successfully' })
  update(@Param('id') id: string, @Body() updateMealDto: UpdateMealDto) {
    return this.mealsService.update(id, updateMealDto);
  }

  @Post(':id/recipes')
  @ApiOperation({ summary: 'Add recipe to meal' })
  @ApiResponse({ status: 201, description: 'Recipe added successfully' })
  addRecipe(@Param('id') id: string, @Body() addRecipeDto: AddRecipeDto) {
    return this.mealsService.addRecipe(id, addRecipeDto);
  }

  @Delete(':id/recipes/:recipeId')
  @ApiOperation({ summary: 'Remove recipe from meal' })
  @ApiResponse({ status: 200, description: 'Recipe removed successfully' })
  removeRecipe(@Param('id') id: string, @Param('recipeId') recipeId: string) {
    return this.mealsService.removeRecipe(id, recipeId);
  }

  @Delete(':id')
  @ApiOperation({ summary: 'Delete meal' })
  @ApiResponse({ status: 200, description: 'Meal deleted successfully' })
  remove(@Param('id') id: string) {
    return this.mealsService.remove(id);
  }
}