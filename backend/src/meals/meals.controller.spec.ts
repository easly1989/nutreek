import { Test, TestingModule } from '@nestjs/testing';
import { MealsController } from './meals.controller';
import { MealsService } from './meals.service';
import { PrismaService } from '../prisma.service';

describe('MealsController', () => {
  let controller: MealsController;
  let service: MealsService;

  beforeEach(async () => {
    const mockPrismaService = {
      meal: {
        create: jest.fn(),
        findMany: jest.fn(),
        findUnique: jest.fn(),
        update: jest.fn(),
        delete: jest.fn(),
      },
      mealRecipe: {
        create: jest.fn(),
        findMany: jest.fn(),
        findUnique: jest.fn(),
        update: jest.fn(),
        delete: jest.fn(),
      },
    };

    const module: TestingModule = await Test.createTestingModule({
      controllers: [MealsController],
      providers: [
        MealsService,
        {
          provide: PrismaService,
          useValue: mockPrismaService,
        },
      ],
    }).compile();

    controller = module.get<MealsController>(MealsController);
    service = module.get<MealsService>(MealsService);
  });

  it('should be defined', () => {
    expect(controller).toBeDefined();
  });

  describe('create', () => {
    it('should create a new meal', async () => {
      const createMealDto = { type: 'Breakfast', dayId: 'day1' };
      const mockResult = { id: '1', type: 'Breakfast', dayId: 'day1' };

      jest.spyOn(service, 'create').mockResolvedValue(mockResult as any);

      const result = await controller.create('day1', createMealDto);
      expect(result).toEqual(mockResult);
    });
  });

  describe('findAll', () => {
    it('should return all meals for a day', async () => {
      const mockResult = [{ id: '1', type: 'Breakfast', dayId: 'day1' }];

      jest.spyOn(service, 'findAll').mockResolvedValue(mockResult as any);

      const result = await controller.findAll('day1');
      expect(result).toEqual(mockResult);
    });
  });

  describe('addRecipe', () => {
    it('should add a recipe to a meal', async () => {
      const addRecipeDto = { recipeId: 'recipe1' };
      const mockResult = { id: '1', mealId: 'meal1', recipeId: 'recipe1' };

      jest.spyOn(service, 'addRecipe').mockResolvedValue(mockResult as any);

      const result = await controller.addRecipe('meal1', addRecipeDto);
      expect(result).toEqual(mockResult);
    });
  });

  describe('update', () => {
    it('should update a meal', async () => {
      const updateMealDto = { type: 'Lunch' };
      const mockResult = { id: '1', type: 'Lunch', dayId: 'day1' };

      jest.spyOn(service, 'update').mockResolvedValue(mockResult as any);

      const result = await controller.update('meal1', updateMealDto);
      expect(result).toEqual(mockResult);
    });
  });

  describe('remove', () => {
    it('should remove a meal', async () => {
      const mockResult = { id: '1', type: 'Breakfast', dayId: 'day1' };

      jest.spyOn(service, 'remove').mockResolvedValue(mockResult as any);

      const result = await controller.remove('meal1');
      expect(result).toEqual(mockResult);
    });
  });
});