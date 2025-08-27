import { Injectable } from '@nestjs/common';
import { PrismaService } from '../prisma.service';

export interface NutritionSummary {
  totalCalories: number;
  totalProtein: number;
  totalCarbs: number;
  totalFat: number;
  averageDailyCalories: number;
  averageDailyProtein: number;
  averageDailyCarbs: number;
  averageDailyFat: number;
}

export interface RecipeStats {
  totalRecipes: number;
  averageIngredientsPerRecipe: number;
  mostUsedIngredients: Array<{
    name: string;
    count: number;
  }>;
  nutritionalDistribution: {
    highProtein: number;
    lowCalorie: number;
    vegetarian: number;
  };
}

export interface HouseholdStats {
  totalMembers: number;
  weeklyPlansCreated: number;
  averagePlanDuration: number;
  mostActiveDay: string;
  recipeUsage: Array<{
    recipeName: string;
    usageCount: number;
  }>;
}

@Injectable()
export class AnalyticsService {
  constructor(private prisma: PrismaService) {}

  async getNutritionSummary(tenantId: string, startDate?: Date, endDate?: Date): Promise<NutritionSummary> {
    const weeklyPlans = await this.prisma.weeklyPlan.findMany({
      where: {
        tenantId,
        ...(startDate && endDate && {
          startDate: {
            gte: startDate,
            lte: endDate,
          },
        }),
      },
      include: {
        days: {
          include: {
            meals: {
              include: {
                recipes: {
                  include: {
                    ingredients: true,
                  },
                },
              },
            },
          },
        },
      },
    });

    let totalCalories = 0;
    let totalProtein = 0;
    let totalCarbs = 0;
    let totalFat = 0;
    let totalDays = 0;

    for (const plan of weeklyPlans) {
      for (const day of plan.days) {
        totalDays++;
        for (const meal of day.meals) {
          for (const recipe of meal.recipes) {
            if (recipe.macros) {
              totalCalories += recipe.macros.kcal || 0;
              totalProtein += recipe.macros.protein || 0;
              totalCarbs += recipe.macros.carbs || 0;
              totalFat += recipe.macros.fat || 0;
            }
          }
        }
      }
    }

    return {
      totalCalories,
      totalProtein,
      totalCarbs,
      totalFat,
      averageDailyCalories: totalDays > 0 ? totalCalories / totalDays : 0,
      averageDailyProtein: totalDays > 0 ? totalProtein / totalDays : 0,
      averageDailyCarbs: totalDays > 0 ? totalCarbs / totalDays : 0,
      averageDailyFat: totalDays > 0 ? totalFat / totalDays : 0,
    };
  }

  async getRecipeStats(tenantId: string): Promise<RecipeStats> {
    const recipes = await this.prisma.recipe.findMany({
      where: {
        meal: {
          day: {
            weeklyPlan: {
              tenantId,
            },
          },
        },
      },
      include: {
        ingredients: true,
      },
    });

    const ingredientUsage = new Map<string, number>();
    let totalIngredients = 0;

    for (const recipe of recipes) {
      totalIngredients += recipe.ingredients.length;
      for (const ingredient of recipe.ingredients) {
        const current = ingredientUsage.get(ingredient.name) || 0;
        ingredientUsage.set(ingredient.name, current + 1);
      }
    }

    const mostUsedIngredients = Array.from(ingredientUsage.entries())
      .sort(([, a], [, b]) => b - a)
      .slice(0, 10)
      .map(([name, count]) => ({ name, count }));

    return {
      totalRecipes: recipes.length,
      averageIngredientsPerRecipe: recipes.length > 0 ? totalIngredients / recipes.length : 0,
      mostUsedIngredients,
      nutritionalDistribution: {
        highProtein: recipes.filter(r => (r.macros?.protein || 0) > 20).length,
        lowCalorie: recipes.filter(r => (r.macros?.kcal || 0) < 300).length,
        vegetarian: recipes.length, // Placeholder - would need ingredient analysis
      },
    };
  }

  async getHouseholdStats(tenantId: string): Promise<HouseholdStats> {
    const [memberships, weeklyPlans, recipeUsage] = await Promise.all([
      this.prisma.membership.findMany({
        where: { tenantId },
      }),
      this.prisma.weeklyPlan.findMany({
        where: { tenantId },
        include: { days: true },
      }),
      this.prisma.recipe.findMany({
        where: {
          meal: {
            day: {
              weeklyPlan: {
                tenantId,
              },
            },
          },
        },
        select: {
          name: true,
          meal: {
            select: {
              day: {
                select: {
                  weeklyPlan: true,
                },
              },
            },
          },
        },
      }),
    ]);

    const dayUsage = new Map<string, number>();
    let totalDays = 0;

    for (const plan of weeklyPlans) {
      totalDays += plan.days.length;
      for (const day of plan.days) {
        const dayName = new Date(day.date).toLocaleDateString('en-US', { weekday: 'long' });
        const current = dayUsage.get(dayName) || 0;
        dayUsage.set(dayName, current + 1);
      }
    }

    const mostActiveDay = Array.from(dayUsage.entries())
      .sort(([, a], [, b]) => b - a)[0]?.[0] || 'N/A';

    const recipeCount = new Map<string, number>();
    for (const recipe of recipeUsage) {
      const current = recipeCount.get(recipe.name) || 0;
      recipeCount.set(recipe.name, current + 1);
    }

    const topRecipes = Array.from(recipeCount.entries())
      .sort(([, a], [, b]) => b - a)
      .slice(0, 5)
      .map(([recipeName, usageCount]) => ({ recipeName, usageCount }));

    return {
      totalMembers: memberships.length,
      weeklyPlansCreated: weeklyPlans.length,
      averagePlanDuration: weeklyPlans.length > 0 ? totalDays / weeklyPlans.length : 0,
      mostActiveDay,
      recipeUsage: topRecipes,
    };
  }

  async getWeeklyTrends(tenantId: string, weeks: number = 4): Promise<any[]> {
    const endDate = new Date();
    const startDate = new Date();
    startDate.setDate(endDate.getDate() - (weeks * 7));

    const weeklyPlans = await this.prisma.weeklyPlan.findMany({
      where: {
        tenantId,
        startDate: {
          gte: startDate,
          lte: endDate,
        },
      },
      include: {
        days: {
          include: {
            meals: {
              include: {
                recipes: true,
              },
            },
          },
        },
      },
      orderBy: { startDate: 'asc' },
    });

    return weeklyPlans.map(plan => {
      let totalMeals = 0;
      let totalRecipes = 0;

      for (const day of plan.days) {
        for (const meal of day.meals) {
          totalMeals++;
          totalRecipes += meal.recipes.length;
        }
      }

      return {
        week: plan.startDate.toISOString().split('T')[0],
        totalMeals,
        totalRecipes,
        averageMealsPerDay: plan.days.length > 0 ? totalMeals / plan.days.length : 0,
      };
    });
  }
}