import { Test, TestingModule } from '@nestjs/testing';
import { RecipesController } from './recipes.controller';
import { RecipesService } from './recipes.service';

describe('RecipesController', () => {
  let controller: RecipesController;
  let service: RecipesService;

  beforeEach(async () => {
    const module: TestingModule = await Test.createTestingModule({
      controllers: [RecipesController],
      providers: [RecipesService],
    }).compile();

    controller = module.get<RecipesController>(RecipesController);
    service = module.get<RecipesService>(RecipesService);
  });

  it('should be defined', () => {
    expect(controller).toBeDefined();
  });

  describe('create', () => {
    it('should create a new recipe', async () => {
      const createRecipeDto = { name: 'Chicken Stir Fry', macros: { kcal: 300, protein: 25, carbs: 20, fat: 15 } };
      const mockResult = { id: '1', name: 'Chicken Stir Fry', macros: { kcal: 300, protein: 25, carbs: 20, fat: 15 } };

      jest.spyOn(service, 'create').mockResolvedValue(mockResult as any);

      const result = await controller.create(createRecipeDto);
      expect(result).toEqual(mockResult);
    });
  });

  describe('findAll', () => {
    it('should return all recipes', async () => {
      const mockResult = [{ id: '1', name: 'Chicken Stir Fry' }];

      jest.spyOn(service, 'findAll').mockResolvedValue(mockResult as any);

      const result = await controller.findAll();
      expect(result).toEqual(mockResult);
    });
  });

  describe('search', () => {
    it('should search recipes by query', async () => {
      const mockResult = [{ id: '1', name: 'Chicken Stir Fry' }];

      jest.spyOn(service, 'search').mockResolvedValue(mockResult as any);

      const result = await controller.search('chicken');
      expect(result).toEqual(mockResult);
    });
  });

  describe('addIngredient', () => {
    it('should add ingredient to recipe', async () => {
      const addIngredientDto = { name: 'Chicken Breast', unit: 'g', amount: 200 };
      const mockResult = { id: '1', name: 'Chicken Breast', unit: 'g', amount: 200, recipeId: 'recipe1' };

      jest.spyOn(service, 'addIngredient').mockResolvedValue(mockResult as any);

      const result = await controller.addIngredient('recipe1', addIngredientDto);
      expect(result).toEqual(mockResult);
    });
  });

  describe('update', () => {
    it('should update a recipe', async () => {
      const updateRecipeDto = { name: 'Updated Recipe' };
      const mockResult = { id: '1', name: 'Updated Recipe' };

      jest.spyOn(service, 'update').mockResolvedValue(mockResult as any);

      const result = await controller.update('recipe1', updateRecipeDto);
      expect(result).toEqual(mockResult);
    });
  });

  describe('remove', () => {
    it('should remove a recipe', async () => {
      const mockResult = { id: '1', name: 'Chicken Stir Fry' };

      jest.spyOn(service, 'remove').mockResolvedValue(mockResult as any);

      const result = await controller.remove('recipe1');
      expect(result).toEqual(mockResult);
    });
  });
});