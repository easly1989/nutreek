import { Injectable } from '@nestjs/common';
import { PrismaService } from '../prisma.service';

export interface CreateWeeklyPlanDto {
  startDate: Date;
}

export interface UpdateWeeklyPlanDto {
  startDate?: Date;
}

// UI Metadata interfaces
export interface UIMetadata {
  colors: {
    primary: string;
    secondary: string;
    accent: string;
    gradient: string;
  };
  icons: {
    breakfast: string;
    snack: string;
    lunch: string;
    dinner: string;
  };
  displayNames: {
    breakfast: string;
    snack: string;
    lunch: string;
    dinner: string;
  };
}

export interface MealWithUIMetadata {
  id: string;
  type: string;
  recipes?: any[];
  uiMetadata: {
    color: string;
    icon: string;
    displayName: string;
    gradient: string;
  };
}

export interface DayWithUIMetadata {
  id: string;
  date: string;
  meals?: MealWithUIMetadata[];
  uiMetadata: {
    dayName: string;
    displayDate: string;
    shortName: string;
    color: string;
  };
}

export interface WeeklyPlanWithUIMetadata {
  id: string;
  tenantId: string;
  startDate: string;
  days?: DayWithUIMetadata[];
  uiMetadata: {
    weekDisplay: string;
    totalDays: number;
    completedDays: number;
    color: string;
    gradient: string;
  };
}

@Injectable()
export class PlansService {
  constructor(private prisma: PrismaService) {}

  // UI Metadata generation methods
  private generateUIMetadata(): UIMetadata {
    return {
      colors: {
        primary: 'hsl(142, 76%, 36%)',
        secondary: 'hsl(45, 93%, 47%)',
        accent: 'hsl(199, 89%, 48%)',
        gradient: 'linear-gradient(135deg, hsl(142, 76%, 36%) 0%, hsl(199, 89%, 48%) 100%)'
      },
      icons: {
        breakfast: '🌅',
        snack: '🍎',
        lunch: '☀️',
        dinner: '🌙'
      },
      displayNames: {
        breakfast: 'Breakfast',
        snack: 'Snack',
        lunch: 'Lunch',
        dinner: 'Dinner'
      }
    };
  }

  private getMealTypeMetadata(mealType: string) {
    const metadataMap = {
      'Breakfast': {
        color: 'hsl(45, 93%, 47%)',
        icon: '🌅',
        displayName: 'Breakfast',
        gradient: 'linear-gradient(135deg, hsl(45, 93%, 47%) 0%, hsl(25, 95%, 53%) 100%)'
      },
      'Snack': {
        color: 'hsl(142, 76%, 36%)',
        icon: '🍎',
        displayName: 'Snack',
        gradient: 'linear-gradient(135deg, hsl(142, 76%, 36%) 0%, hsl(120, 60%, 50%) 100%)'
      },
      'Lunch': {
        color: 'hsl(199, 89%, 48%)',
        icon: '☀️',
        displayName: 'Lunch',
        gradient: 'linear-gradient(135deg, hsl(199, 89%, 48%) 0%, hsl(220, 89%, 56%) 100%)'
      },
      'Dinner': {
        color: 'hsl(280, 100%, 70%)',
        icon: '🌙',
        displayName: 'Dinner',
        gradient: 'linear-gradient(135deg, hsl(280, 100%, 70%) 0%, hsl(320, 100%, 75%) 100%)'
      }
    };
    return metadataMap[mealType as keyof typeof metadataMap] || metadataMap['Breakfast'];
  }

  private getDayMetadata(date: Date, index: number) {
    const colors = [
      'hsl(142, 76%, 36%)',
      'hsl(199, 89%, 48%)',
      'hsl(280, 100%, 70%)',
      'hsl(25, 95%, 53%)',
      'hsl(45, 93%, 47%)',
      'hsl(320, 100%, 75%)',
      'hsl(220, 89%, 56%)'
    ];

    return {
      dayName: date.toLocaleDateString('en-US', { weekday: 'long' }),
      displayDate: date.toLocaleDateString('en-US', { month: 'short', day: 'numeric' }),
      shortName: date.toLocaleDateString('en-US', { weekday: 'short' }),
      color: colors[index % colors.length]
    };
  }

  private transformMealWithUIMetadata(meal: any): MealWithUIMetadata {
    const metadata = this.getMealTypeMetadata(meal.type);
    return {
      ...meal,
      uiMetadata: metadata
    };
  }

  private transformDayWithUIMetadata(day: any, index: number): DayWithUIMetadata {
    const date = new Date(day.date);
    const dayMetadata = this.getDayMetadata(date, index);

    return {
      ...day,
      meals: day.meals?.map(meal => this.transformMealWithUIMetadata(meal)),
      uiMetadata: dayMetadata
    };
  }

  private transformWeeklyPlanWithUIMetadata(plan: any): WeeklyPlanWithUIMetadata {
    const startDate = new Date(plan.startDate);
    const completedDays = plan.days?.filter(day => day.meals && day.meals.length > 0).length || 0;

    return {
      ...plan,
      days: plan.days?.map((day, index) => this.transformDayWithUIMetadata(day, index)),
      uiMetadata: {
        weekDisplay: `Week of ${startDate.toLocaleDateString('en-US', {
          month: 'long',
          day: 'numeric',
          year: 'numeric'
        })}`,
        totalDays: plan.days?.length || 0,
        completedDays,
        color: 'hsl(142, 76%, 36%)',
        gradient: 'linear-gradient(135deg, hsl(142, 76%, 36%) 0%, hsl(199, 89%, 48%) 100%)'
      }
    };
  }

  async create(tenantId: string, createWeeklyPlanDto: CreateWeeklyPlanDto) {
    return this.prisma.weeklyPlan.create({
      data: {
        tenantId,
        startDate: createWeeklyPlanDto.startDate,
      },
    });
  }

  async findAll(tenantId: string): Promise<WeeklyPlanWithUIMetadata[]> {
    const plans = await this.prisma.weeklyPlan.findMany({
      where: { tenantId },
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

    return plans.map(plan => this.transformWeeklyPlanWithUIMetadata(plan));
  }

  async findOne(id: string): Promise<WeeklyPlanWithUIMetadata | null> {
    const plan = await this.prisma.weeklyPlan.findUnique({
      where: { id },
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

    return plan ? this.transformWeeklyPlanWithUIMetadata(plan) : null;
  }

  async update(id: string, updateWeeklyPlanDto: UpdateWeeklyPlanDto) {
    return this.prisma.weeklyPlan.update({
      where: { id },
      data: updateWeeklyPlanDto,
    });
  }

  async remove(id: string) {
    return this.prisma.weeklyPlan.delete({
      where: { id },
    });
  }

  // Get UI metadata configuration for frontend
  getUIMetadata(): UIMetadata {
    return this.generateUIMetadata();
  }

  // Get meal type configurations
  getMealTypeConfigs() {
    return {
      breakfast: this.getMealTypeMetadata('Breakfast'),
      snack: this.getMealTypeMetadata('Snack'),
      lunch: this.getMealTypeMetadata('Lunch'),
      dinner: this.getMealTypeMetadata('Dinner')
    };
  }
}