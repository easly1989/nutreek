import { Injectable } from '@nestjs/common';
import { PrismaService } from '../prisma.service';

export interface ShoppingListItem {
  ingredient: {
    id: string;
    name: string;
    unit: string;
  };
  totalAmount: number;
  recipes: Array<{
    id: string;
    name: string;
    amount: number;
  }>;
}

export interface ShoppingList {
  id: string;
  tenantId: string;
  weeklyPlanId: string;
  items: ShoppingListItem[];
  createdAt: Date;
  updatedAt: Date;
}

@Injectable()
export class ShoppingListsService {
  constructor(private prisma: PrismaService) {}

  async generateFromWeeklyPlan(weeklyPlanId: string): Promise<ShoppingList> {
    // Get the weekly plan with all meals and recipes
    const weeklyPlan = await this.prisma.weeklyPlan.findUnique({
      where: { id: weeklyPlanId },
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

    if (!weeklyPlan) {
      throw new Error('Weekly plan not found');
    }

    // Aggregate ingredients from all recipes
    const ingredientMap = new Map<string, ShoppingListItem>();

    for (const day of weeklyPlan.days) {
      for (const meal of day.meals) {
        for (const recipe of meal.recipes) {
          for (const ingredient of recipe.ingredients) {
            const key = `${ingredient.name}-${ingredient.unit}`;
            const existing = ingredientMap.get(key);

            if (existing) {
              existing.totalAmount += ingredient.amount;
              existing.recipes.push({
                id: recipe.id,
                name: recipe.name,
                amount: ingredient.amount,
              });
            } else {
              ingredientMap.set(key, {
                ingredient: {
                  id: ingredient.id,
                  name: ingredient.name,
                  unit: ingredient.unit,
                },
                totalAmount: ingredient.amount,
                recipes: [{
                  id: recipe.id,
                  name: recipe.name,
                  amount: ingredient.amount,
                }],
              });
            }
          }
        }
      }
    }

    const items = Array.from(ingredientMap.values());

    // Create or update shopping list
    const shoppingList = await this.prisma.shoppingList.upsert({
      where: {
        weeklyPlanId,
      },
      update: {
        items: items,
        updatedAt: new Date(),
      },
      create: {
        tenantId: weeklyPlan.tenantId,
        weeklyPlanId,
        items: items,
      },
    });

    return {
      ...shoppingList,
      items,
    } as ShoppingList;
  }

  async findByWeeklyPlan(weeklyPlanId: string): Promise<ShoppingList | null> {
    const shoppingList = await this.prisma.shoppingList.findUnique({
      where: { weeklyPlanId },
    });

    if (!shoppingList) {
      return null;
    }

    return {
      ...shoppingList,
      items: shoppingList.items as ShoppingListItem[],
    } as ShoppingList;
  }

  async updateItemStatus(
    weeklyPlanId: string,
    ingredientId: string,
    purchased: boolean
  ): Promise<void> {
    const shoppingList = await this.prisma.shoppingList.findUnique({
      where: { weeklyPlanId },
    });

    if (!shoppingList) {
      throw new Error('Shopping list not found');
    }

    const items = shoppingList.items as ShoppingListItem[];
    const updatedItems = items.map(item => {
      if (item.ingredient.id === ingredientId) {
        return {
          ...item,
          purchased,
        };
      }
      return item;
    });

    await this.prisma.shoppingList.update({
      where: { weeklyPlanId },
      data: {
        items: updatedItems,
      },
    });
  }

  async getByTenant(tenantId: string): Promise<ShoppingList[]> {
    const shoppingLists = await this.prisma.shoppingList.findMany({
      where: { tenantId },
      orderBy: { createdAt: 'desc' },
    });

    return shoppingLists.map(list => ({
      ...list,
      items: list.items as ShoppingListItem[],
    })) as ShoppingList[];
  }
}