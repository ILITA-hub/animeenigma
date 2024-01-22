import { Module } from '@nestjs/common';
import { JwtModule } from '@nestjs/jwt';
import { PassportModule } from '@nestjs/passport';
import { AuthService } from './auth.service';
import { config } from '../config/index';
import { JwtStrategy } from './strategies/jwt.strategy';
import { LocalStrategy } from './strategies/local.strategy';
import { SocketStrategy } from './strategies/socket.strategy';

@Module({
  imports: [
    PassportModule.register({ 
      session: true 
    }),
    JwtModule.register({
      secret: config.jwtSecret,
      // signOptions: { expiresIn: '60s' },
    }),
  ],
  providers: [AuthService, JwtStrategy, LocalStrategy, SocketStrategy],
  exports: [AuthService],
})
export class AuthModule {}