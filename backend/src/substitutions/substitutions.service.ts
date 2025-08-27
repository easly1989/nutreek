import { Injectable } from '@nestjs/common';
import { PrismaService } from '../prisma.service';

export interface CreateSubstitutionDto {
  originalId: string;
  substituteId: string;
}

@Injectable()
export class SubstitutionsService {
  constructor(private prisma: PrismaService) {}

  async create(createSubstitutionDto: CreateSubstitutionDto, userId: string) {
    return this.prisma.substitution.create({
      data: {
        ...createSubstitutionDto,
        userId,
      },
    });
  }

  async findAll(userId: string) {
    return this.prisma.substitution.findMany({
      where: { userId },
      include: {
        user: true,
      },
    });
  }

  async findOne(id: string) {
    return this.prisma.substitution.findUnique({
      where: { id },
      include: {
        user: true,
      },
    });
  }

  async remove(id: string) {
    return this.prisma.substitution.delete({
      where: { id },
    });
  }
}