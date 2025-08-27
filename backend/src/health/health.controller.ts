import { Controller, Get } from '@nestjs/common';
import { ApiTags, ApiOperation, ApiResponse } from '@nestjs/swagger';
import { PrismaService } from '../prisma.service';
import { RedisService } from '../redis/redis.service';

@ApiTags('health')
@Controller('health')
export class HealthController {
  constructor(
    private prisma: PrismaService,
    private redis: RedisService,
  ) {}

  @Get()
  @ApiOperation({ summary: 'Get system health status' })
  @ApiResponse({ status: 200, description: 'System is healthy' })
  async getHealth() {
    const health = {
      status: 'ok',
      timestamp: new Date().toISOString(),
      services: {
        database: 'unknown',
        redis: 'unknown',
      },
      uptime: process.uptime(),
      memory: process.memoryUsage(),
      version: process.version,
    };

    try {
      // Check database connection
      await this.prisma.$queryRaw`SELECT 1`;
      health.services.database = 'healthy';
    } catch (error) {
      health.services.database = 'unhealthy';
      health.status = 'degraded';
    }

    try {
      // Check Redis connection
      const redisClient = this.redis.getClient();
      await redisClient.ping();
      health.services.redis = 'healthy';
    } catch (error) {
      health.services.redis = 'unhealthy';
      health.status = 'degraded';
    }

    return health;
  }

  @Get('detailed')
  @ApiOperation({ summary: 'Get detailed system health information' })
  @ApiResponse({ status: 200, description: 'Detailed system information' })
  async getDetailedHealth() {
    const health = await this.getHealth();

    // Add more detailed information
    const detailedHealth = {
      ...health,
      system: {
        platform: process.platform,
        arch: process.arch,
        nodeVersion: process.version,
        environment: process.env.NODE_ENV || 'development',
      },
      process: {
        pid: process.pid,
        uptime: process.uptime(),
        memory: process.memoryUsage(),
        cpuUsage: process.cpuUsage(),
      },
      database: {
        connectionStatus: health.services.database,
        // Add more database stats if needed
      },
      cache: {
        connectionStatus: health.services.redis,
        // Add Redis stats if needed
      },
    };

    return detailedHealth;
  }
}