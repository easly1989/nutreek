import { Controller, Get, Post, Body, Param, Query } from '@nestjs/common';
import { ApiTags, ApiOperation, ApiResponse } from '@nestjs/swagger';
import { I18nService } from './i18n.service';

@ApiTags('i18n')
@Controller('i18n')
export class I18nController {
  constructor(private readonly i18nService: I18nService) {}

  @Get('languages')
  @ApiOperation({ summary: 'Get list of supported languages' })
  @ApiResponse({ status: 200, description: 'List of supported languages' })
  getSupportedLanguages() {
    return {
      languages: this.i18nService.getSupportedLanguages(),
      current: this.i18nService.getCurrentLanguage(),
    };
  }

  @Post('language/:lng')
  @ApiOperation({ summary: 'Change current language' })
  @ApiResponse({ status: 200, description: 'Language changed successfully' })
  async changeLanguage(@Param('lng') lng: string) {
    if (!this.i18nService.isLanguageSupported(lng)) {
      return {
        success: false,
        message: this.i18nService.tError('validationError'),
      };
    }

    await this.i18nService.changeLanguage(lng);
    return {
      success: true,
      message: this.i18nService.t('languageChanged', lng),
      language: lng,
    };
  }

  @Get('translate/:key')
  @ApiOperation({ summary: 'Get translation for a key' })
  @ApiResponse({ status: 200, description: 'Translation retrieved successfully' })
  translate(
    @Param('key') key: string,
    @Query('lng') lng?: string,
    @Query() query?: any,
  ) {
    const options = { lng, ...query };
    return {
      key,
      translation: this.i18nService.translate(key, options),
      language: lng || this.i18nService.getCurrentLanguage(),
    };
  }

  @Get('translations/:namespace')
  @ApiOperation({ summary: 'Get all translations for a namespace' })
  @ApiResponse({ status: 200, description: 'Translations retrieved successfully' })
  getTranslations(
    @Param('namespace') namespace: string,
    @Query('lng') lng?: string,
  ) {
    const language = lng || this.i18nService.getCurrentLanguage();
    const translations = this.i18nService.getTranslationsForNamespace(namespace, language);

    return {
      namespace,
      language,
      translations,
    };
  }
}