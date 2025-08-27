import { Injectable } from '@nestjs/common';
import { PrismaService } from '../prisma.service';

export interface CreateWeeklyPlanDto {
  startDate: Date;
}

export interface UpdateWeeklyPlanDto {
  startDate?: Date;
}

@Injectable()
export class PlansService {
  constructor(private prisma: PrismaService) {}

  async create(tenantId: string, createWeeklyPlanDto: CreateWeeklyPlanDto) {
    return this.prisma.weeklyPlan.create({
      data: {
        tenantId,
        startDate: createWeeklyPlanDto.startDate,
      },
    });
  }

  async findAll(tenantId: string) {
    return this.prisma.weeklyPlan.findMany({
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
  }

  async findOne(id: string) {
    return this.prisma.weeklyPlan.findUnique({
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
}