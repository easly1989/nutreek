import { Injectable } from '@nestjs/common';
import { PrismaService } from '../prisma.service';

export interface NutritionPattern {
  pattern: 'balanced' | 'high_protein' | 'low_carb' | 'vegetarian' | 'keto';
  confidence: number;
  description: string;
  recommendations: string[];
}

export interface MealRecommendation {
  mealType: string;
  recipeName: string;
  nutritionalFit: number;
  reasoning: string;
  alternatives: string[];
}

export interface WeeklyPlanPrediction {
  predictedCalories: number;
  macronutrientDistribution: {
    protein: number;
    carbs: number;
    fat: number;
  };
  confidence: number;
  riskFactors: string[];
  suggestions: string[];
}

export interface UserPreferences {
  dietaryRestrictions: string[];
  allergies: string[];
  preferredCuisines: string[];
  dislikedIngredients: string[];
  nutritionalGoals?: {
    targetCalories?: number;
    targetProtein?: number;
    targetCarbs?: number;
    targetFat?: number;
  };
}

@Injectable()
export class MlInsightsService {
  constructor(private prisma: PrismaService) {}

  async analyzeNutritionPatterns(tenantId: string, userId?: string): Promise<NutritionPattern> {
    // Get historical nutrition data
    const nutritionData = await this.getHistoricalNutritionData(tenantId, userId);

    // Analyze patterns using statistical methods
    const patterns = this.detectNutritionPatterns(nutritionData);

    return {
      pattern: patterns.primaryPattern,
      confidence: patterns.confidence,
      description: this.generatePatternDescription(patterns.primaryPattern),
      recommendations: this.generateRecommendations(patterns.primaryPattern, patterns.gaps),
    };
  }

  async generateMealRecommendations(
    tenantId: string,
    userPreferences: UserPreferences,
    currentWeekData?: any
  ): Promise<MealRecommendation[]> {
    // Get available recipes
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
        meal: true,
      },
    });

    // Analyze user preferences and constraints
    const compatibleRecipes = this.filterRecipesByPreferences(recipes, userPreferences);

    // Generate recommendations based on nutritional balance
    const recommendations = await this.calculateOptimalMealCombinations(
      compatibleRecipes,
      userPreferences.nutritionalGoals
    );

    return recommendations.slice(0, 7); // Return top 7 recommendations
  }

  async predictWeeklyNutrition(tenantId: string, planData: any): Promise<WeeklyPlanPrediction> {
    // Calculate predicted weekly nutrition
    const predictedNutrition = await this.calculatePredictedNutrition(planData);

    // Assess nutritional balance
    const balanceAnalysis = this.analyzeNutritionalBalance(predictedNutrition);

    // Identify risk factors
    const riskFactors = this.identifyRiskFactors(predictedNutrition, balanceAnalysis);

    // Generate suggestions
    const suggestions = this.generateImprovementSuggestions(riskFactors, balanceAnalysis);

    return {
      predictedCalories: predictedNutrition.totalCalories,
      macronutrientDistribution: {
        protein: predictedNutrition.totalProtein,
        carbs: predictedNutrition.totalCarbs,
        fat: predictedNutrition.totalFat,
      },
      confidence: this.calculatePredictionConfidence(planData),
      riskFactors,
      suggestions,
    };
  }

  async analyzeRecipePerformance(tenantId: string): Promise<any> {
    // Get recipe usage statistics
    const recipeStats = await this.prisma.recipe.findMany({
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
        meal: {
          include: {
            day: {
              include: {
                weeklyPlan: true,
              },
            },
          },
        },
      },
    });

    // Analyze recipe popularity and nutritional impact
    const performanceMetrics = this.calculateRecipePerformance(recipeStats);

    // Identify trending ingredients and nutritional patterns
    const trends = this.identifyNutritionalTrends(recipeStats);

    return {
      topRecipes: performanceMetrics.topRecipes,
      nutritionalTrends: trends,
      ingredientPopularity: performanceMetrics.ingredientPopularity,
      mealTypePreferences: performanceMetrics.mealTypePreferences,
    };
  }

  async generatePersonalizedInsights(userId: string, tenantId: string): Promise<any> {
    // Get user's nutrition history
    const userHistory = await this.getUserNutritionHistory(userId, tenantId);

    // Analyze personal patterns
    const personalPatterns = this.analyzePersonalPatterns(userHistory);

    // Generate personalized recommendations
    const recommendations = this.generatePersonalizedRecommendations(personalPatterns);

    return {
      patterns: personalPatterns,
      recommendations,
      goals: this.assessGoalProgress(userHistory),
      predictions: this.predictFutureNeeds(userHistory),
    };
  }

  // Helper methods
  private async getHistoricalNutritionData(tenantId: string, userId?: string) {
    const plans = await this.prisma.weeklyPlan.findMany({
      where: {
        tenantId,
        startDate: {
          gte: new Date(Date.now() - 90 * 24 * 60 * 60 * 1000), // Last 90 days
        },
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
      orderBy: { startDate: 'desc' },
      take: 12, // Last 12 weeks
    });

    return plans;
  }

  private detectNutritionPatterns(nutritionData: any[]): any {
    if (nutritionData.length === 0) {
      return { primaryPattern: 'balanced', confidence: 0, gaps: [] };
    }

    // Calculate average macronutrient distribution
    const totals = nutritionData.reduce((acc, plan) => {
      // This would contain complex calculation logic
      return acc;
    }, { totalProtein: 0, totalCarbs: 0, totalFat: 0, totalCalories: 0, count: 0 });

    const avgProtein = totals.totalProtein / totals.totalCalories;
    const avgCarbs = totals.totalCarbs / totals.totalCalories;
    const avgFat = totals.totalFat / totals.totalCalories;

    // Determine primary pattern
    let primaryPattern: NutritionPattern['pattern'] = 'balanced';
    let confidence = 0.5;

    if (avgProtein > 0.25) {
      primaryPattern = 'high_protein';
      confidence = 0.8;
    } else if (avgCarbs < 0.3) {
      primaryPattern = 'low_carb';
      confidence = 0.7;
    } else if (avgFat > 0.4) {
      primaryPattern = 'keto';
      confidence = 0.75;
    }

    return {
      primaryPattern,
      confidence,
      gaps: this.identifyNutritionalGaps(avgProtein, avgCarbs, avgFat),
    };
  }

  private generatePatternDescription(pattern: NutritionPattern['pattern']): string {
    const descriptions = {
      balanced: 'Your nutrition shows a well-balanced macronutrient distribution with adequate protein, carbs, and fats.',
      high_protein: 'Your meals are protein-rich, which is excellent for muscle maintenance and satiety.',
      low_carb: 'Your carb intake is relatively low, which may support certain health goals.',
      vegetarian: 'Your meals primarily follow vegetarian guidelines.',
      keto: 'Your nutrition aligns with ketogenic principles with high fat and low carb intake.',
    };

    return descriptions[pattern] || descriptions.balanced;
  }

  private generateRecommendations(pattern: NutritionPattern['pattern'], gaps: string[]): string[] {
    const recommendations: Record<NutritionPattern['pattern'], string[]> = {
      balanced: [
        'Continue maintaining balanced nutrition',
        'Consider adding variety to your meal choices',
        'Monitor portion sizes to maintain balance',
      ],
      high_protein: [
        'Ensure adequate carb intake for energy',
        'Include complex carbohydrates in your meals',
        'Balance protein sources for complete amino acids',
      ],
      low_carb: [
        'Consider adding nutrient-dense carbohydrates',
        'Include fiber-rich foods for digestive health',
        'Monitor energy levels and adjust carb intake if needed',
      ],
      vegetarian: [
        'Ensure adequate protein intake from plant sources',
        'Include iron-rich foods and vitamin C for absorption',
        'Consider B12 supplementation if following strict vegetarian diet',
      ],
      keto: [
        'Monitor electrolyte balance',
        'Include nutrient-dense vegetables',
        'Ensure adequate fiber intake',
      ],
    };

    return [...recommendations[pattern], ...gaps.map(gap => `Address ${gap} deficiency`)];
  }

  private identifyNutritionalGaps(avgProtein: number, avgCarbs: number, avgFat: number): string[] {
    const gaps = [];

    if (avgProtein < 0.1) gaps.push('protein');
    if (avgCarbs < 0.4) gaps.push('carbohydrate');
    if (avgFat < 0.2) gaps.push('healthy fats');

    return gaps;
  }

  private filterRecipesByPreferences(recipes: any[], preferences: UserPreferences): any[] {
    return recipes.filter(recipe => {
      // Check for dietary restrictions
      const hasRestrictedIngredients = recipe.ingredients.some((ingredient: any) =>
        preferences.dislikedIngredients.includes(ingredient.name.toLowerCase()) ||
        preferences.allergies.includes(ingredient.name.toLowerCase())
      );

      // Check for preferred cuisines (if we had cuisine data)
      // const matchesCuisine = !preferences.preferredCuisines.length ||
      //   preferences.preferredCuisines.includes(recipe.cuisine);

      return !hasRestrictedIngredients;
    });
  }

  private async calculateOptimalMealCombinations(recipes: any[], goals?: any): Promise<MealRecommendation[]> {
    // This would contain complex optimization algorithms
    // For now, return simple recommendations based on nutritional balance

    const recommendations: MealRecommendation[] = [];

    // Group recipes by meal type
    const mealGroups = recipes.reduce((acc, recipe) => {
      const mealType = recipe.meal.type;
      if (!acc[mealType]) acc[mealType] = [];
      acc[mealType].push(recipe);
      return acc;
    }, {} as Record<string, any[]>);

    // Generate recommendations for each meal type
    Object.entries(mealGroups).forEach(([mealType, mealRecipes]) => {
      if (Array.isArray(mealRecipes) && mealRecipes.length > 0) {
        const topRecipe = mealRecipes[0]; // Simplified selection

        recommendations.push({
          mealType,
          recipeName: topRecipe.name,
          nutritionalFit: this.calculateNutritionalFit(topRecipe, goals),
          reasoning: `Balanced nutritional profile for ${mealType.toLowerCase()}`,
          alternatives: mealRecipes.slice(1, 4).map((r: any) => r.name),
        });
      }
    });

    return recommendations;
  }

  private calculateNutritionalFit(recipe: any, goals?: any): number {
    if (!goals || !recipe.macros) return 0.75; // Default good fit

    // Calculate how well recipe matches nutritional goals
    const proteinFit = Math.min(recipe.macros.protein / (goals.targetProtein || recipe.macros.protein), 1);
    const carbFit = Math.min(recipe.macros.carbs / (goals.targetCarbs || recipe.macros.carbs), 1);
    const fatFit = Math.min(recipe.macros.fat / (goals.targetFat || recipe.macros.fat), 1);

    return (proteinFit + carbFit + fatFit) / 3;
  }

  private async calculatePredictedNutrition(planData: any): Promise<any> {
    // This would calculate total nutrition for the plan
    return {
      totalCalories: 2100,
      totalProtein: 150,
      totalCarbs: 200,
      totalFat: 70,
    };
  }

  private analyzeNutritionalBalance(nutrition: any): any {
    const totalMacros = nutrition.totalProtein + nutrition.totalCarbs + nutrition.totalFat;
    return {
      proteinPercentage: nutrition.totalProtein / totalMacros,
      carbPercentage: nutrition.totalCarbs / totalMacros,
      fatPercentage: nutrition.totalFat / totalMacros,
    };
  }

  private identifyRiskFactors(nutrition: any, balance: any): string[] {
    const risks = [];

    if (balance.proteinPercentage < 0.15) risks.push('Low protein intake');
    if (balance.carbPercentage < 0.45) risks.push('Low carbohydrate intake');
    if (balance.fatPercentage > 0.4) risks.push('High fat intake');

    return risks;
  }

  private generateImprovementSuggestions(risks: string[], balance: any): string[] {
    const suggestions = [];

    if (risks.includes('Low protein intake')) {
      suggestions.push('Increase protein-rich foods like lean meats, eggs, or legumes');
    }
    if (risks.includes('Low carbohydrate intake')) {
      suggestions.push('Add complex carbohydrates like whole grains and vegetables');
    }
    if (risks.includes('High fat intake')) {
      suggestions.push('Balance fat intake with lean proteins and vegetables');

    }

    return suggestions;
  }

  private calculatePredictionConfidence(planData: any): number {
    // Simple confidence calculation based on plan completeness
    return 0.85; // Placeholder
  }

  private calculateRecipePerformance(recipeStats: any[]): any {
    // Calculate recipe performance metrics
    return {
      topRecipes: [],
      ingredientPopularity: {},
      mealTypePreferences: {},
    };
  }

  private identifyNutritionalTrends(recipeStats: any[]): any {
    // Identify nutritional trends
    return {};
  }

  private async getUserNutritionHistory(userId: string, tenantId: string): Promise<any[]> {
    // Get user's personal nutrition history
    return [];
  }

  private analyzePersonalPatterns(history: any[]): any {
    // Analyze personal eating patterns
    return {};
  }

  private generatePersonalizedRecommendations(patterns: any): any {
    // Generate personalized recommendations
    return {};
  }

  private assessGoalProgress(history: any[]): any {
    // Assess progress towards nutritional goals
    return {};
  }

  private predictFutureNeeds(history: any[]): any {
    // Predict future nutritional needs
    return {};
  }
}