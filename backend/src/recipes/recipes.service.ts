import { Injectable } from '@nestjs/common';
import { PrismaService } from '../prisma.service';

export interface CreateRecipeDto {
  name: string;
  macros?: any;
  source?: string;
  externalId?: string;
}

export interface UpdateRecipeDto {
  name?: string;
  macros?: any;
  source?: string;
  externalId?: string;
}

export interface AddIngredientDto {
  name: string;
  unit: string;
  amount: number;
}

@Injectable()
export class RecipesService {
  constructor(private prisma: PrismaService) {}

  async create(createRecipeDto: CreateRecipeDto) {
    return this.prisma.recipe.create({
      data: createRecipeDto,
    });
  }

  async findAll() {
    return this.prisma.recipe.findMany({
      include: {
        ingredients: true,
        meal: true,
      },
    });
  }

  async search(query: string) {
    return this.prisma.recipe.findMany({
      where: {
        OR: [
          { name: { contains: query, mode: 'insensitive' } },
          { ingredients: { some: { name: { contains: query, mode: 'insensitive' } } } },
        ],
      },
      include: {
        ingredients: true,
        meal: true,
      },
    });
  }

  async findOne(id: string) {
    return this.prisma.recipe.findUnique({
      where: { id },
      include: {
        ingredients: true,
        meal: true,
      },
    });
  }

  async addIngredient(recipeId: string, addIngredientDto: AddIngredientDto) {
    return this.prisma.ingredient.create({
      data: {
        ...addIngredientDto,
        recipeId,
      },
    });
  }

  async removeIngredient(recipeId: string, ingredientId: string) {
    return this.prisma.ingredient.delete({
      where: { id: ingredientId },
    });
  }

  async update(id: string, updateRecipeDto: UpdateRecipeDto) {
    return this.prisma.recipe.update({
      where: { id },
      data: updateRecipeDto,
    });
  }

  async remove(id: string) {
    return this.prisma.recipe.delete({
      where: { id },
    });
  }

  async findByMeal(mealId: string) {
    return this.prisma.recipe.findMany({
      where: { mealId },
      include: {
        ingredients: true,
      },
    });
  }
}