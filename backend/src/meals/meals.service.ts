import { Injectable } from '@nestjs/common';
import { PrismaService } from '../prisma.service';

export interface CreateMealDto {
  type: string;
  dayId: string;
}

export interface UpdateMealDto {
  type?: string;
}

export interface AddRecipeDto {
  recipeId: string;
}

@Injectable()
export class MealsService {
  constructor(private prisma: PrismaService) {}

  async create(dayId: string, createMealDto: Omit<CreateMealDto, 'dayId'>) {
    return this.prisma.meal.create({
      data: {
        type: createMealDto.type,
        dayId,
      },
    });
  }

  async findAll(dayId: string) {
    return this.prisma.meal.findMany({
      where: { dayId },
      include: {
        recipes: {
          include: {
            ingredients: true,
          },
        },
      },
    });
  }

  async findOne(id: string) {
    return this.prisma.meal.findUnique({
      where: { id },
      include: {
        recipes: {
          include: {
            ingredients: true,
          },
        },
      },
    });
  }

  async update(id: string, updateMealDto: UpdateMealDto) {
    return this.prisma.meal.update({
      where: { id },
      data: updateMealDto,
    });
  }

  async addRecipe(mealId: string, addRecipeDto: AddRecipeDto) {
    // Check if recipe already exists for this meal
    const existingRecipe = await this.prisma.recipe.findFirst({
      where: {
        mealId,
        id: addRecipeDto.recipeId,
      },
    });

    if (existingRecipe) {
      throw new Error('Recipe already added to this meal');
    }

    return this.prisma.recipe.update({
      where: { id: addRecipeDto.recipeId },
      data: { mealId },
    });
  }

  async removeRecipe(mealId: string, recipeId: string) {
    return this.prisma.recipe.update({
      where: { id: recipeId },
      data: { mealId: null },
    });
  }

  async remove(id: string) {
    return this.prisma.meal.delete({
      where: { id },
    });
  }
}