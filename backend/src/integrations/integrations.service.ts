import { Injectable } from '@nestjs/common';
import { HttpService } from '@nestjs/axios';
import { ConfigService } from '@nestjs/config';
import { firstValueFrom } from 'rxjs';

export interface NutritionApiResponse {
  foods: Array<{
    food_name: string;
    brand_name?: string;
    serving_qty: number;
    serving_unit: string;
    serving_weight_grams: number;
    nf_calories: number;
    nf_total_fat: number;
    nf_saturated_fat: number;
    nf_cholesterol: number;
    nf_sodium: number;
    nf_total_carbohydrate: number;
    nf_dietary_fiber: number;
    nf_sugars: number;
    nf_protein: number;
    nf_potassium: number;
  }>;
}

export interface WeatherData {
  temperature: number;
  humidity: number;
  conditions: string;
  location: string;
}

export interface CalendarEvent {
  id: string;
  title: string;
  start: Date;
  end: Date;
  location?: string;
  description?: string;
}

export interface SocialMediaPost {
  id: string;
  content: string;
  timestamp: Date;
  platform: 'twitter' | 'facebook' | 'instagram';
  engagement: {
    likes: number;
    shares: number;
    comments: number;
  };
}

@Injectable()
export class IntegrationsService {
  constructor(
    private httpService: HttpService,
    private configService: ConfigService,
  ) {}

  // Nutrition API Integration (Nutritionix/Nutrition Database)
  async searchNutritionData(query: string): Promise<any[]> {
    try {
      const appId = this.configService.get('NUTRITIONIX_APP_ID');
      const appKey = this.configService.get('NUTRITIONIX_APP_KEY');

      if (!appId || !appKey) {
        // Fallback to mock data if API keys are not configured
        return this.getMockNutritionData(query);
      }

      const response = await firstValueFrom(
        this.httpService.get<NutritionApiResponse>(
          `https://trackapi.nutritionix.com/v2/search/instant?query=${encodeURIComponent(query)}`,
          {
            headers: {
              'x-app-id': appId,
              'x-app-key': appKey,
            },
          }
        )
      );

      return this.transformNutritionData(response.data as NutritionApiResponse);
    } catch (error) {
      console.error('Nutrition API error:', error);
      return this.getMockNutritionData(query);
    }
  }

  // Weather API Integration
  async getWeatherData(location: string): Promise<WeatherData> {
    try {
      const apiKey = this.configService.get('OPENWEATHER_API_KEY');

      if (!apiKey) {
        return this.getMockWeatherData(location);
      }

      const response = await firstValueFrom(
        this.httpService.get(
          `https://api.openweathermap.org/data/2.5/weather?q=${encodeURIComponent(location)}&appid=${apiKey}&units=metric`
        )
      );

      return this.transformWeatherData(response.data as any);
    } catch (error) {
      console.error('Weather API error:', error);
      return this.getMockWeatherData(location);
    }
  }

  // Calendar Integration (Google Calendar/Outlook)
  async getCalendarEvents(userId: string, startDate: Date, endDate: Date): Promise<CalendarEvent[]> {
    try {
      // This would integrate with Google Calendar API or Microsoft Graph API
      // For now, return mock data
      return this.getMockCalendarEvents(userId, startDate, endDate);
    } catch (error) {
      console.error('Calendar API error:', error);
      return [];
    }
  }

  // Social Media Integration
  async getSocialMediaPosts(userId: string, platforms: string[]): Promise<SocialMediaPost[]> {
    try {
      const posts: SocialMediaPost[] = [];

      for (const platform of platforms) {
        const platformPosts = await this.getPlatformPosts(userId, platform);
        posts.push(...platformPosts);
      }

      return posts.sort((a, b) => b.timestamp.getTime() - a.timestamp.getTime());
    } catch (error) {
      console.error('Social media API error:', error);
      return [];
    }
  }

  // Fitness Tracker Integration
  async getFitnessData(userId: string, date: Date): Promise<any> {
    try {
      // This would integrate with Fitbit, Garmin, Apple Health, etc.
      return this.getMockFitnessData(userId, date);
    } catch (error) {
      console.error('Fitness API error:', error);
      return this.getMockFitnessData(userId, date);
    }
  }

  // Recipe Import from External Sources
  async importRecipeFromUrl(url: string): Promise<any> {
    try {
      // This would scrape recipe websites or use APIs like Spoonacular
      const response = await firstValueFrom(
        this.httpService.get(url)
      );

      return this.parseRecipeFromHtml(response.data as string);
    } catch (error) {
      console.error('Recipe import error:', error);
      throw new Error('Failed to import recipe from URL');
    }
  }

  // Barcode Scanner Integration
  async getProductByBarcode(barcode: string): Promise<any> {
    try {
      // This would use barcode lookup APIs
      const apiKey = this.configService.get('BARCODE_API_KEY');

      if (!apiKey) {
        return this.getMockBarcodeData(barcode);
      }

      const response = await firstValueFrom(
        this.httpService.get(
          `https://api.barcodelookup.com/v2/products?barcode=${barcode}&key=${apiKey}`
        )
      );

      return this.transformBarcodeData(response.data as any);
    } catch (error) {
      console.error('Barcode API error:', error);
      return this.getMockBarcodeData(barcode);
    }
  }

  // Voice Assistant Integration
  async processVoiceCommand(userId: string, audioData: Buffer, tenantId: string): Promise<any> {
    try {
      // This would integrate with Google Speech-to-Text and Dialogflow
      const transcription = await this.transcribeAudio(audioData);
      const intent = await this.analyzeIntent(transcription, userId, tenantId);

      return this.executeVoiceCommand(intent, userId, tenantId);
    } catch (error) {
      console.error('Voice processing error:', error);
      throw new Error('Failed to process voice command');
    }
  }

  // Smart Home Integration
  async updateSmartDevices(userId: string, nutritionData: any): Promise<void> {
    try {
      // This would integrate with smart home devices (scales, refrigerators, etc.)
      const devices = await this.getUserSmartDevices(userId);

      for (const device of devices) {
        await this.updateDeviceNutritionData(device, nutritionData);
      }
    } catch (error) {
      console.error('Smart home integration error:', error);
    }
  }

  // Email/SMS Notifications
  async sendNotification(userId: string, type: string, data: any): Promise<void> {
    try {
      const user = await this.getUserContactInfo(userId);

      switch (type) {
        case 'meal_reminder':
          await this.sendMealReminder(user, data);
          break;
        case 'shopping_list':
          await this.sendShoppingList(user, data);
          break;
        case 'nutrition_report':
          await this.sendNutritionReport(user, data);
          break;
        default:
          console.log(`Unknown notification type: ${type}`);
      }
    } catch (error) {
      console.error('Notification error:', error);
    }
  }

  // Helper methods for mock data and transformations
  private transformNutritionData(data: NutritionApiResponse): any[] {
    return data.foods.map(food => ({
      name: food.food_name,
      brand: food.brand_name,
      serving: {
        quantity: food.serving_qty,
        unit: food.serving_unit,
        weight: food.serving_weight_grams,
      },
      nutrition: {
        calories: food.nf_calories,
        fat: food.nf_total_fat,
        saturatedFat: food.nf_saturated_fat,
        cholesterol: food.nf_cholesterol,
        sodium: food.nf_sodium,
        carbs: food.nf_total_carbohydrate,
        fiber: food.nf_dietary_fiber,
        sugar: food.nf_sugars,
        protein: food.nf_protein,
        potassium: food.nf_potassium,
      },
    }));
  }

  private getMockNutritionData(query: string): any[] {
    return [
      {
        name: `${query} (Mock Data)`,
        brand: 'Generic Brand',
        serving: { quantity: 100, unit: 'g', weight: 100 },
        nutrition: {
          calories: 150,
          fat: 5,
          saturatedFat: 2,
          cholesterol: 10,
          sodium: 200,
          carbs: 20,
          fiber: 3,
          sugar: 5,
          protein: 8,
          potassium: 150,
        },
      },
    ];
  }

  private transformWeatherData(data: any): WeatherData {
    return {
      temperature: data.main.temp,
      humidity: data.main.humidity,
      conditions: data.weather[0].description,
      location: data.name,
    };
  }

  private getMockWeatherData(location: string): WeatherData {
    return {
      temperature: 22,
      humidity: 65,
      conditions: 'partly cloudy',
      location,
    };
  }

  private getMockCalendarEvents(userId: string, startDate: Date, endDate: Date): CalendarEvent[] {
    return [
      {
        id: 'event-1',
        title: 'Team Meeting',
        start: new Date(startDate.getTime() + 9 * 60 * 60 * 1000),
        end: new Date(startDate.getTime() + 10 * 60 * 60 * 1000),
        location: 'Conference Room A',
        description: 'Weekly team sync',
      },
    ];
  }

  private async getPlatformPosts(userId: string, platform: string): Promise<SocialMediaPost[]> {
    // Mock social media posts
    return [
      {
        id: 'post-1',
        content: 'Just had an amazing healthy breakfast! 🥑🍅',
        timestamp: new Date(),
        platform: platform as any,
        engagement: { likes: 15, shares: 3, comments: 5 },
      },
    ];
  }

  private getMockFitnessData(userId: string, date: Date): any {
    return {
      steps: 8432,
      caloriesBurned: 420,
      activeMinutes: 45,
      workouts: [
        {
          type: 'running',
          duration: 30,
          calories: 280,
        },
      ],
    };
  }

  private parseRecipeFromHtml(html: string): any {
    // This would use cheerio or similar to parse recipe data
    return {
      title: 'Parsed Recipe',
      ingredients: ['ingredient 1', 'ingredient 2'],
      instructions: ['step 1', 'step 2'],
    };
  }

  private getMockBarcodeData(barcode: string): any {
    return {
      productName: 'Sample Product',
      brand: 'Sample Brand',
      nutrition: {
        calories: 100,
        fat: 3,
        carbs: 15,
        protein: 5,
      },
    };
  }

  private async transcribeAudio(audioData: Buffer): Promise<string> {
    // Mock transcription
    return 'add chicken breast to my shopping list';
  }

  private async analyzeIntent(transcription: string, userId: string, tenantId: string): Promise<any> {
    // Mock intent analysis
    return {
      intent: 'add_to_shopping_list',
      entities: {
        item: 'chicken breast',
      },
    };
  }

  private async executeVoiceCommand(intent: any, userId: string, tenantId: string): Promise<any> {
    // Mock command execution
    return {
      success: true,
      message: 'Added chicken breast to your shopping list',
    };
  }

  private async getUserSmartDevices(userId: string): Promise<any[]> {
    // Mock smart devices
    return [
      {
        id: 'scale-1',
        type: 'smart_scale',
        connected: true,
      },
    ];
  }

  private async updateDeviceNutritionData(device: any, nutritionData: any): Promise<void> {
    // Mock device update
    console.log(`Updating device ${device.id} with nutrition data`);
  }

  private async getUserContactInfo(userId: string): Promise<any> {
    // Mock user contact info
    return {
      email: 'user@example.com',
      phone: '+1234567890',
    };
  }

  private async sendMealReminder(user: any, data: any): Promise<void> {
    // Mock email/SMS sending
    console.log(`Sending meal reminder to ${user.email}`);
  }

  private async sendShoppingList(user: any, data: any): Promise<void> {
    // Mock shopping list sending
    console.log(`Sending shopping list to ${user.email}`);
  }

  private async sendNutritionReport(user: any, data: any): Promise<void> {
    // Mock nutrition report sending
    console.log(`Sending nutrition report to ${user.email}`);
  }

  private transformBarcodeData(data: any): any {
    // Transform barcode API response
    return data;
  }
}