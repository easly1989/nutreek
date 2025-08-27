import { Injectable } from '@nestjs/common';
import { PrismaService } from '../prisma.service';
import { RedisService } from '../redis/redis.service';

export interface CreateIngredientDto {
  name: string;
  unit: string;
  amount: number;
  macros?: any;
  source?: string;
  externalId?: string;
}

export interface UpdateQuantityDto {
  quantity: number;
}

@Injectable()
export class IngredientsService {
  constructor(
    private prisma: PrismaService,
    private redisService: RedisService,
  ) {}

  private get redisClient() {
    return this.redisService.getClient();
  }

  async search(query: string): Promise<any[]> {
    const cacheKey = `ingredient_search:${query}`;

    // Try to get from cache first
    const cachedResult = await this.redisClient.get(cacheKey);
    if (cachedResult) {
      return JSON.parse(cachedResult);
    }

    // Search local database first
    let results = await this.prisma.ingredient.findMany({
      where: {
        name: {
          contains: query,
          mode: 'insensitive',
        },
      },
      include: {
        recipe: true,
      },
    });

    // If no local results, try FatSecret API (mock implementation)
    if (results.length === 0) {
      results = await this.searchFatSecret(query);
    }

    // Cache the results for 1 hour
    await this.redisClient.setex(cacheKey, 3600, JSON.stringify(results));

    return results;
  }

  private async searchFatSecret(query: string): Promise<any[]> {
    // This is a mock implementation
    // In real implementation, this would call FatSecret API
    // For now, we'll return mock data and simulate API rate limiting
    const mockResults = [
      {
        id: 'external_1',
        name: `${query} (from FatSecret)`,
        unit: 'g',
        amount: 100,
        macros: { kcal: 165, protein: 31, carbs: 0, fat: 3.6 },
        source: 'fatsecret',
        externalId: '12345',
      },
    ];

    // Check API quota (mock implementation)
    const quotaKey = 'fatsecret_quota';
    const quota = await this.redisClient.get(quotaKey);

    if (quota && parseInt(quota) >= 5000) {
      // Return stale cache if quota exceeded
      const staleCacheKey = `stale_ingredient_search:${query}`;
      const staleResult = await this.redisClient.get(staleCacheKey);
      if (staleResult) {
        return JSON.parse(staleResult);
      }
    }

    // Increment quota
    await this.redisClient.incr(quotaKey);

    return mockResults;
  }

  async create(createIngredientDto: CreateIngredientDto) {
    return this.prisma.ingredient.create({
      data: createIngredientDto,
    });
  }

  async findAll() {
    return this.prisma.ingredient.findMany({
      include: {
        recipe: true,
      },
    });
  }

  async findOne(id: string) {
    return this.prisma.ingredient.findUnique({
      where: { id },
      include: {
        recipe: true,
      },
    });
  }

  async updateQuantity(ingredientId: string, updateQuantityDto: UpdateQuantityDto, userId: string) {
    return this.prisma.userIngredientQuantity.upsert({
      where: {
        userId_ingredientId: {
          userId,
          ingredientId,
        },
      },
      update: {
        quantity: updateQuantityDto.quantity,
      },
      create: {
        userId,
        ingredientId,
        quantity: updateQuantityDto.quantity,
      },
    });
  }

  async getUserQuantities(userId: string) {
    return this.prisma.userIngredientQuantity.findMany({
      where: { userId },
      include: {
        ingredient: true,
      },
    });
  }

  async update(id: string, updateIngredientDto: Partial<CreateIngredientDto>) {
    return this.prisma.ingredient.update({
      where: { id },
      data: updateIngredientDto,
    });
  }

  async remove(id: string) {
    return this.prisma.ingredient.delete({
      where: { id },
    });
  }
}