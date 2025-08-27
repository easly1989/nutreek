import { Module } from '@nestjs/common';
import { SubstitutionsService } from './substitutions.service';
import { SubstitutionsController } from './substitutions.controller';
import { PrismaModule } from '../prisma.module';

@Module({
  imports: [PrismaModule],
  controllers: [SubstitutionsController],
  providers: [SubstitutionsService],
  exports: [SubstitutionsService],
})
export class SubstitutionsModule {}