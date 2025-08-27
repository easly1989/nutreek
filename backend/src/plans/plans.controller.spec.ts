import { Test, TestingModule } from '@nestjs/testing';
import { PlansController } from './plans.controller';
import { PlansService } from './plans.service';
import { PrismaService } from '../prisma.service';

describe('PlansController', () => {
  let controller: PlansController;
  let service: PlansService;

  beforeEach(async () => {
    const mockPrismaService = {
      plan: {
        create: jest.fn(),
        findMany: jest.fn(),
        findUnique: jest.fn(),
        update: jest.fn(),
        delete: jest.fn(),
      },
      meal: {
        findMany: jest.fn(),
      },
    };

    const module: TestingModule = await Test.createTestingModule({
      controllers: [PlansController],
      providers: [
        PlansService,
        {
          provide: PrismaService,
          useValue: mockPrismaService,
        },
      ],
    }).compile();

    controller = module.get<PlansController>(PlansController);
    service = module.get<PlansService>(PlansService);
  });

  it('should be defined', () => {
    expect(controller).toBeDefined();
  });

  describe('create', () => {
    it('should create a new weekly plan', async () => {
      const createPlanDto = { startDate: new Date() };
      const mockResult = { id: '1', tenantId: 'tenant1', startDate: new Date() };
      
      jest.spyOn(service, 'create').mockResolvedValue(mockResult as any);
      
      const result = await controller.create('tenant1', createPlanDto);
      expect(result).toEqual(mockResult);
    });
  });

  describe('findOne', () => {
    it('should return a weekly plan with meals', async () => {
      const mockResult = { id: '1', tenantId: 'tenant1', startDate: new Date(), days: [] };
      
      jest.spyOn(service, 'findOne').mockResolvedValue(mockResult as any);
      
      const result = await controller.findOne('plan1');
      expect(result).toEqual(mockResult);
    });
  });

  describe('update', () => {
    it('should update a weekly plan', async () => {
      const updatePlanDto = { startDate: new Date() };
      const mockResult = { id: '1', tenantId: 'tenant1', startDate: new Date() };
      
      jest.spyOn(service, 'update').mockResolvedValue(mockResult as any);
      
      const result = await controller.update('plan1', updatePlanDto);
      expect(result).toEqual(mockResult);
    });
  });
});