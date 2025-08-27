import { Test, TestingModule } from '@nestjs/testing';
import { TenantsController } from './tenants.controller';
import { TenantsService } from './tenants.service';

describe('TenantsController', () => {
  let controller: TenantsController;
  let service: TenantsService;

  beforeEach(async () => {
    const module: TestingModule = await Test.createTestingModule({
      controllers: [TenantsController],
      providers: [TenantsService],
    }).compile();

    controller = module.get<TenantsController>(TenantsController);
    service = module.get<TenantsService>(TenantsService);
  });

  it('should be defined', () => {
    expect(controller).toBeDefined();
  });

  describe('create', () => {
    it('should create a new tenant', async () => {
      const createTenantDto = { name: 'My Household' };
      const mockResult = { id: '1', name: 'My Household' };
      
      jest.spyOn(service, 'create').mockResolvedValue(mockResult as any);
      
      const result = await controller.create(createTenantDto, { user: { id: 'user1' } } as any);
      expect(result).toEqual(mockResult);
    });
  });

  describe('findAll', () => {
    it('should return all tenants for user', async () => {
      const mockResult = [{ id: '1', name: 'My Household' }];
      
      jest.spyOn(service, 'findAll').mockResolvedValue(mockResult as any);
      
      const result = await controller.findAll({ user: { id: 'user1' } } as any);
      expect(result).toEqual(mockResult);
    });
  });

  describe('invite', () => {
    it('should invite a member to tenant', async () => {
      const inviteDto = { email: 'member@example.com', role: 'member' as const };
      const mockResult = { id: '1', userId: 'user2', tenantId: 'tenant1', role: 'member' };

      jest.spyOn(service, 'invite').mockResolvedValue(mockResult as any);

      const result = await controller.invite('tenant1', inviteDto);
      expect(result).toEqual(mockResult);
    });
  });
});