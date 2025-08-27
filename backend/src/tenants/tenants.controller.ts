import { Controller, Get, Post, Body, Param, Req, UseGuards } from '@nestjs/common';
import { ApiTags, ApiOperation, ApiResponse } from '@nestjs/swagger';
import { TenantsService, CreateTenantDto, InviteMemberDto } from './tenants.service';
import { JwtAuthGuard } from '../auth/jwt-auth.guard';

@ApiTags('tenants')
@Controller('tenants')
@UseGuards(JwtAuthGuard)
export class TenantsController {
  constructor(private readonly tenantsService: TenantsService) {}

  @Post()
  @ApiOperation({ summary: 'Create a new tenant/household' })
  @ApiResponse({ status: 201, description: 'Tenant created successfully' })
  create(@Body() createTenantDto: CreateTenantDto, @Req() req) {
    return this.tenantsService.create(createTenantDto, req.user.id);
  }

  @Get()
  @ApiOperation({ summary: 'Get all tenants for current user' })
  @ApiResponse({ status: 200, description: 'List of tenants' })
  findAll(@Req() req) {
    return this.tenantsService.findAll(req.user.id);
  }

  @Get(':id')
  @ApiOperation({ summary: 'Get tenant by ID' })
  @ApiResponse({ status: 200, description: 'Tenant details' })
  findOne(@Param('id') id: string) {
    return this.tenantsService.findOne(id);
  }

  @Post(':id/invite')
  @ApiOperation({ summary: 'Invite a member to tenant' })
  @ApiResponse({ status: 201, description: 'Member invited successfully' })
  invite(@Param('id') id: string, @Body() inviteMemberDto: InviteMemberDto) {
    return this.tenantsService.invite(id, inviteMemberDto);
  }
}