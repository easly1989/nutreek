import { Controller, Get, Post, Body, Delete, Req, UseGuards } from '@nestjs/common';
import { ApiTags, ApiOperation, ApiResponse } from '@nestjs/swagger';
import { SubstitutionsService, CreateSubstitutionDto } from './substitutions.service';
import { JwtAuthGuard } from '../auth/jwt-auth.guard';

@ApiTags('substitutions')
@Controller('users/substitutions')
@UseGuards(JwtAuthGuard)
export class SubstitutionsController {
  constructor(private readonly substitutionsService: SubstitutionsService) {}

  @Post()
  @ApiOperation({ summary: 'Create a new substitution' })
  @ApiResponse({ status: 201, description: 'Substitution created successfully' })
  create(@Body() createSubstitutionDto: CreateSubstitutionDto, @Req() req) {
    return this.substitutionsService.create(createSubstitutionDto, req.user.id);
  }

  @Get()
  @ApiOperation({ summary: 'Get all substitutions for user' })
  @ApiResponse({ status: 200, description: 'List of substitutions' })
  findAll(@Req() req) {
    return this.substitutionsService.findAll(req.user.id);
  }

  @Delete(':id')
  @ApiOperation({ summary: 'Delete substitution' })
  @ApiResponse({ status: 200, description: 'Substitution deleted successfully' })
  remove(@Req() req, id: string) {
    return this.substitutionsService.remove(id);
  }
}