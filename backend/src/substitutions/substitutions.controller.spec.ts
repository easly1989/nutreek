import { Test, TestingModule } from '@nestjs/testing';
import { SubstitutionsController } from './substitutions.controller';
import { SubstitutionsService } from './substitutions.service';
import { PrismaService } from '../prisma.service';

describe('SubstitutionsController', () => {
  let controller: SubstitutionsController;
  let service: SubstitutionsService;

  beforeEach(async () => {
    const mockPrismaService = {
      substitution: {
        create: jest.fn(),
        findMany: jest.fn(),
        findUnique: jest.fn(),
        update: jest.fn(),
        delete: jest.fn(),
      },
    };

    const module: TestingModule = await Test.createTestingModule({
      controllers: [SubstitutionsController],
      providers: [
        SubstitutionsService,
        {
          provide: PrismaService,
          useValue: mockPrismaService,
        },
      ],
    }).compile();

    controller = module.get<SubstitutionsController>(SubstitutionsController);
    service = module.get<SubstitutionsService>(SubstitutionsService);
  });

  it('should be defined', () => {
    expect(controller).toBeDefined();
  });

  describe('create', () => {
    it('should create a new substitution', async () => {
      const createSubstitutionDto = { originalId: 'ing1', substituteId: 'ing2' };
      const mockResult = { id: '1', userId: 'user1', originalId: 'ing1', substituteId: 'ing2' };

      jest.spyOn(service, 'create').mockResolvedValue(mockResult as any);

      const result = await controller.create(createSubstitutionDto, { user: { id: 'user1' } } as any);
      expect(result).toEqual(mockResult);
    });
  });

  describe('findAll', () => {
    it('should return all substitutions for user', async () => {
      const mockResult = [{ id: '1', originalId: 'ing1', substituteId: 'ing2' }];

      jest.spyOn(service, 'findAll').mockResolvedValue(mockResult as any);

      const result = await controller.findAll({ user: { id: 'user1' } } as any);
      expect(result).toEqual(mockResult);
    });
  });

  describe('remove', () => {
    it('should remove a substitution', async () => {
      const mockResult = { id: '1', originalId: 'ing1', substituteId: 'ing2' };

      jest.spyOn(service, 'remove').mockResolvedValue(mockResult as any);

      const result = await controller.remove({ user: { id: 'user1' } } as any, 'sub1');
      expect(result).toEqual(mockResult);
    });
  });
});