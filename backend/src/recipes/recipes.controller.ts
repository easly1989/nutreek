import { Controller, Get, Post, Body, Patch, Param, Delete, Query } from '@nestjs/common';
import { ApiTags, ApiOperation, ApiResponse } from '@nestjs/swagger';
import { RecipesService, CreateRecipeDto, UpdateRecipeDto, AddIngredientDto } from './recipes.service';

@ApiTags('recipes')
@Controller('recipes')
export class RecipesController {
  constructor(private readonly recipesService: RecipesService) {}

  @Post()
  @ApiOperation({ summary: 'Create a new recipe' })
  @ApiResponse({ status: 201, description: 'Recipe created successfully' })
  create(@Body() createRecipeDto: CreateRecipeDto) {
    return this.recipesService.create(createRecipeDto);
  }

  @Get()
  @ApiOperation({ summary: 'Get all recipes' })
  @ApiResponse({ status: 200, description: 'List of recipes' })
  findAll() {
    return this.recipesService.findAll();
  }

  @Get('search')
  @ApiOperation({ summary: 'Search recipes' })
  @ApiResponse({ status: 200, description: 'Search results' })
  search(@Query('q') query: string) {
    return this.recipesService.search(query);
  }

  @Get(':id')
  @ApiOperation({ summary: 'Get recipe by ID' })
  @ApiResponse({ status: 200, description: 'Recipe details' })
  findOne(@Param('id') id: string) {
    return this.recipesService.findOne(id);
  }

  @Post(':id/ingredients')
  @ApiOperation({ summary: 'Add ingredient to recipe' })
  @ApiResponse({ status: 201, description: 'Ingredient added successfully' })
  addIngredient(@Param('id') id: string, @Body() addIngredientDto: AddIngredientDto) {
    return this.recipesService.addIngredient(id, addIngredientDto);
  }

  @Delete(':id/ingredients/:ingredientId')
  @ApiOperation({ summary: 'Remove ingredient from recipe' })
  @ApiResponse({ status: 200, description: 'Ingredient removed successfully' })
  removeIngredient(@Param('id') id: string, @Param('ingredientId') ingredientId: string) {
    return this.recipesService.removeIngredient(id, ingredientId);
  }

  @Patch(':id')
  @ApiOperation({ summary: 'Update recipe' })
  @ApiResponse({ status: 200, description: 'Recipe updated successfully' })
  update(@Param('id') id: string, @Body() updateRecipeDto: UpdateRecipeDto) {
    return this.recipesService.update(id, updateRecipeDto);
  }

  @Delete(':id')
  @ApiOperation({ summary: 'Delete recipe' })
  @ApiResponse({ status: 200, description: 'Recipe deleted successfully' })
  remove(@Param('id') id: string) {
    return this.recipesService.remove(id);
  }
}