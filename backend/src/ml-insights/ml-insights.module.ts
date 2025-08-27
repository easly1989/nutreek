import { Module } from '@nestjs/common';
import { MlInsightsService } from './ml-insights.service';
import { MlInsightsController } from './ml-insights.controller';
import { PrismaModule } from '../prisma.module';

@Module({
  imports: [PrismaModule],
  controllers: [MlInsightsController],
  providers: [MlInsightsService],
  exports: [MlInsightsService],
})
export class MlInsightsModule {}