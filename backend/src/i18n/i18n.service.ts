import { Injectable } from '@nestjs/common';
import { ConfigService } from '@nestjs/config';
import * as fs from 'fs';
import * as path from 'path';

interface TranslationOptions {
  lng?: string;
  [key: string]: any;
}

@Injectable()
export class I18nService {
  private translations: Map<string, any> = new Map();
  private currentLanguage = 'en';

  constructor(private configService: ConfigService) {
    this.loadTranslations();
  }

  private loadTranslations() {
    const supportedLanguages = this.getSupportedLanguages();
    const namespaces = ['common', 'auth', 'nutrition', 'errors'];

    for (const lng of supportedLanguages) {
      const lngTranslations: any = {};

      for (const ns of namespaces) {
        const filePath = path.join(__dirname, `../../assets/i18n/${lng}/${ns}.json`);
        try {
          if (fs.existsSync(filePath)) {
            const fileContent = fs.readFileSync(filePath, 'utf8');
            lngTranslations[ns] = JSON.parse(fileContent);
          } else {
            // Create default translation files if they don't exist
            this.createDefaultTranslations(lng, ns);
            lngTranslations[ns] = this.getDefaultTranslations(ns);
          }
        } catch (error) {
          console.error(`Error loading translation file ${filePath}:`, error);
          lngTranslations[ns] = this.getDefaultTranslations(ns);
        }
      }

      this.translations.set(lng, lngTranslations);
    }
  }

  private createDefaultTranslations(lng: string, ns: string) {
    const dirPath = path.join(__dirname, `../../assets/i18n/${lng}`);
    if (!fs.existsSync(dirPath)) {
      fs.mkdirSync(dirPath, { recursive: true });
    }

    const filePath = path.join(dirPath, `${ns}.json`);
    const defaultTranslations = this.getDefaultTranslations(ns);
    fs.writeFileSync(filePath, JSON.stringify(defaultTranslations, null, 2));
  }

  private getDefaultTranslations(ns: string): any {
    switch (ns) {
      case 'common':
        return {
          welcome: 'Welcome',
          save: 'Save',
          cancel: 'Cancel',
          delete: 'Delete',
          edit: 'Edit',
          add: 'Add',
          search: 'Search',
          loading: 'Loading...',
          error: 'Error',
          success: 'Success',
        };
      case 'auth':
        return {
          login: 'Login',
          logout: 'Logout',
          register: 'Register',
          email: 'Email',
          password: 'Password',
          confirmPassword: 'Confirm Password',
          forgotPassword: 'Forgot Password?',
          invalidCredentials: 'Invalid email or password',
          userNotFound: 'User not found',
          emailAlreadyExists: 'Email already exists',
        };
      case 'nutrition':
        return {
          calories: 'Calories',
          protein: 'Protein',
          carbs: 'Carbohydrates',
          fat: 'Fat',
          fiber: 'Fiber',
          sugar: 'Sugar',
          sodium: 'Sodium',
          potassium: 'Potassium',
          meal: 'Meal',
          recipe: 'Recipe',
          ingredient: 'Ingredient',
          nutritionFacts: 'Nutrition Facts',
          dailyGoals: 'Daily Goals',
          weeklyPlan: 'Weekly Plan',
        };
      case 'errors':
        return {
          serverError: 'Internal server error',
          validationError: 'Validation error',
          notFound: 'Resource not found',
          unauthorized: 'Unauthorized access',
          forbidden: 'Access forbidden',
          networkError: 'Network error',
        };
      default:
        return {};
    }
  }

  translate(key: string, options: TranslationOptions = {}): string {
    const lng = options.lng || this.currentLanguage;
    const translations = this.translations.get(lng);

    if (!translations) {
      return key;
    }

    // Split key by namespace (e.g., 'auth.login' -> namespace: 'auth', key: 'login')
    const [namespace, ...keyParts] = key.split('.');
    const actualKey = keyParts.join('.') || namespace;

    let translation = translations[namespace]?.[actualKey] || translations.common?.[key] || key;

    // Simple interpolation
    if (options && typeof translation === 'string') {
      Object.keys(options).forEach(param => {
        if (param !== 'lng') {
          translation = translation.replace(new RegExp(`\\$\\{${param}\\}`, 'g'), options[param]);
        }
      });
    }

    return translation;
  }

  getCurrentLanguage(): string {
    return this.currentLanguage;
  }

  async changeLanguage(lng: string): Promise<void> {
    if (this.getSupportedLanguages().includes(lng)) {
      this.currentLanguage = lng;
    }
  }

  getSupportedLanguages(): string[] {
    return ['en', 'es', 'fr', 'de', 'it', 'pt'];
  }

  isLanguageSupported(lng: string): boolean {
    return this.getSupportedLanguages().includes(lng);
  }

  // Convenience methods for common translations
  t(key: string, lng?: string): string {
    return this.translate(key, { lng });
  }

  tAuth(key: string, lng?: string): string {
    return this.translate(`auth.${key}`, { lng });
  }

  tNutrition(key: string, lng?: string): string {
    return this.translate(`nutrition.${key}`, { lng });
  }

  tError(key: string, lng?: string): string {
    return this.translate(`errors.${key}`, { lng });
  }

  getTranslationsForNamespace(namespace: string, lng?: string): any {
    const language = lng || this.currentLanguage;
    const translations = this.translations.get(language);
    return translations?.[namespace] || {};
  }
}