import { Test, TestingModule } from '@nestjs/testing';
import { IngredientsController } from './ingredients.controller';
import { IngredientsService } from './ingredients.service';

describe('IngredientsController', () => {
  let controller: IngredientsController;
  let service: IngredientsService;

  beforeEach(async () => {
    const module: TestingModule = await Test.createTestingModule({
      controllers: [IngredientsController],
      providers: [IngredientsService],
    }).compile();

    controller = module.get<IngredientsController>(IngredientsController);
    service = module.get<IngredientsService>(IngredientsService);
  });

  it('should be defined', () => {
    expect(controller).toBeDefined();
  });

  describe('search', () => {
    it('should search ingredients', async () => {
      const mockResult = [{ id: '1', name: 'Chicken Breast', unit: 'g', amount: 100 }];

      jest.spyOn(service, 'search').mockResolvedValue(mockResult as any);

      const result = await controller.search('chicken');
      expect(result).toEqual(mockResult);
    });
  });

  describe('create', () => {
    it('should create custom ingredient', async () => {
      const createIngredientDto = { name: 'Custom Spice', unit: 'g', amount: 5, macros: { kcal: 20, protein: 1, carbs: 3, fat: 0.5 } };
      const mockResult = { id: '1', name: 'Custom Spice', unit: 'g', amount: 5 };

      jest.spyOn(service, 'create').mockResolvedValue(mockResult as any);

      const result = await controller.create(createIngredientDto);
      expect(result).toEqual(mockResult);
    });
  });

  describe('updateQuantity', () => {
    it('should update user-specific ingredient quantity', async () => {
      const updateQuantityDto = { quantity: 150 };
      const mockResult = { id: '1', userId: 'user1', ingredientId: 'ing1', quantity: 150 };

      jest.spyOn(service, 'updateQuantity').mockResolvedValue(mockResult as any);

      const result = await controller.updateQuantity('ing1', updateQuantityDto, { user: { id: 'user1' } } as any);
      expect(result).toEqual(mockResult);
    });
  });

  describe('findAll', () => {
    it('should return all ingredients', async () => {
      const mockResult = [{ id: '1', name: 'Chicken Breast' }];

      jest.spyOn(service, 'findAll').mockResolvedValue(mockResult as any);

      const result = await controller.findAll();
      expect(result).toEqual(mockResult);
    });
  });

  describe('findOne', () => {
    it('should return ingredient by ID', async () => {
      const mockResult = { id: '1', name: 'Chicken Breast' };

      jest.spyOn(service, 'findOne').mockResolvedValue(mockResult as any);

      const result = await controller.findOne('1');
      expect(result).toEqual(mockResult);
    });
  });
});