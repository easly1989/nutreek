import { Controller, Get, Post, Body, Patch, Param, Query, Req, UseGuards } from '@nestjs/common';
import { ApiTags, ApiOperation, ApiResponse } from '@nestjs/swagger';
import { IngredientsService, CreateIngredientDto, UpdateQuantityDto } from './ingredients.service';
import { JwtAuthGuard } from '../auth/jwt-auth.guard';

@ApiTags('ingredients')
@Controller('ingredients')
export class IngredientsController {
  constructor(private readonly ingredientsService: IngredientsService) {}

  @Get('search')
  @ApiOperation({ summary: 'Search ingredients' })
  @ApiResponse({ status: 200, description: 'Search results' })
  search(@Query('q') query: string) {
    return this.ingredientsService.search(query);
  }

  @Post()
  @ApiOperation({ summary: 'Create custom ingredient' })
  @ApiResponse({ status: 201, description: 'Ingredient created successfully' })
  create(@Body() createIngredientDto: CreateIngredientDto) {
    return this.ingredientsService.create(createIngredientDto);
  }

  @Get()
  @ApiOperation({ summary: 'Get all ingredients' })
  @ApiResponse({ status: 200, description: 'List of ingredients' })
  findAll() {
    return this.ingredientsService.findAll();
  }

  @Get(':id')
  @ApiOperation({ summary: 'Get ingredient by ID' })
  @ApiResponse({ status: 200, description: 'Ingredient details' })
  findOne(@Param('id') id: string) {
    return this.ingredientsService.findOne(id);
  }

  @Patch(':id/quantity')
  @UseGuards(JwtAuthGuard)
  @ApiOperation({ summary: 'Update user-specific ingredient quantity' })
  @ApiResponse({ status: 200, description: 'Quantity updated successfully' })
  updateQuantity(@Param('id') id: string, @Body() updateQuantityDto: UpdateQuantityDto, @Req() req) {
    return this.ingredientsService.updateQuantity(id, updateQuantityDto, req.user.id);
  }

  @Patch(':id')
  @ApiOperation({ summary: 'Update ingredient' })
  @ApiResponse({ status: 200, description: 'Ingredient updated successfully' })
  update(@Param('id') id: string, @Body() updateIngredientDto: Partial<CreateIngredientDto>) {
    return this.ingredientsService.update(id, updateIngredientDto);
  }
}