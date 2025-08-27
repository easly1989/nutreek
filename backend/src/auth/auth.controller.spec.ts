import { Test, TestingModule } from '@nestjs/testing';
import { AuthController } from './auth.controller';
import { AuthService } from './auth.service';
import { PrismaService } from '../prisma.service';
import { JwtService } from '@nestjs/jwt';

describe('AuthController', () => {
  let controller: AuthController;
  let service: AuthService;

  beforeEach(async () => {
    const mockPrismaService = {
      user: {
        create: jest.fn(),
        findUnique: jest.fn(),
        findFirst: jest.fn(),
        update: jest.fn(),
        delete: jest.fn(),
      },
    };

    const mockJwtService = {
      sign: jest.fn(),
      verify: jest.fn(),
    };

    const module: TestingModule = await Test.createTestingModule({
      controllers: [AuthController],
      providers: [
        AuthService,
        {
          provide: PrismaService,
          useValue: mockPrismaService,
        },
        {
          provide: JwtService,
          useValue: mockJwtService,
        },
      ],
    }).compile();

    controller = module.get<AuthController>(AuthController);
    service = module.get<AuthService>(AuthService);
  });

  it('should be defined', () => {
    expect(controller).toBeDefined();
  });

  describe('register', () => {
    it('should register a new user', async () => {
      const registerDto = { email: 'test@example.com', name: 'Test User' };
      const mockResult = {
        access_token: 'token',
        user: { id: '1', email: 'test@example.com', name: 'Test User' },
      };
      
      jest.spyOn(service, 'register').mockResolvedValue(mockResult);
      
      const result = await controller.register(registerDto);
      expect(result).toEqual(mockResult);
      expect(service.register).toHaveBeenCalledWith(registerDto);
    });
  });

  describe('login', () => {
    it('should login user and return token', async () => {
      const loginDto = { email: 'test@example.com' };
      const mockResult = {
        access_token: 'token',
        user: { id: '1', email: 'test@example.com' },
      };
      
      jest.spyOn(service, 'login').mockResolvedValue(mockResult);
      
      const result = await controller.login(loginDto);
      expect(result).toEqual(mockResult);
      expect(service.login).toHaveBeenCalledWith(loginDto);
    });
  });

  describe('me', () => {
    it('should return current user info', async () => {
      const req = { user: { id: '1', email: 'test@example.com' } };
      const result = await controller.me(req as any);
      expect(result).toEqual(req.user);
    });
  });
});