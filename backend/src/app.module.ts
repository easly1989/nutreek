import { Module } from '@nestjs/common';
import { ConfigModule } from '@nestjs/config';
import { AppController } from './app.controller';
import { AppService } from './app.service';
import { PrismaModule } from './prisma.module';
import { AuthModule } from './auth/auth.module';
import { TenantsModule } from './tenants/tenants.module';
import { PlansModule } from './plans/plans.module';
import { MealsModule } from './meals/meals.module';
import { RecipesModule } from './recipes/recipes.module';
import { RedisModule } from './redis/redis.module';
import { IngredientsModule } from './ingredients/ingredients.module';
import { SubstitutionsModule } from './substitutions/substitutions.module';
import { ShoppingListsModule } from './shopping-lists/shopping-lists.module';

@Module({
  imports: [
    ConfigModule.forRoot({
      isGlobal: true,
    }),
    PrismaModule,
    RedisModule,
    AuthModule,
    TenantsModule,
    PlansModule,
    MealsModule,
    RecipesModule,
    IngredientsModule,
    SubstitutionsModule,
    ShoppingListsModule,
  ],
  controllers: [AppController],
  providers: [AppService],
})
export class AppModule {}