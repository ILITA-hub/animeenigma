import { 
  ExecutionContext,
  Injectable,
  UnauthorizedException, 
} from '@nestjs/common';
import { AuthGuard } from '@nestjs/passport';

@Injectable()
export class SocketAuthGuard extends AuthGuard('socket') {
  canActivate(context: ExecutionContext) {
    const request = context.switchToHttp().getRequest();

    if (!request.user) {
      return false;
    }

    return super.canActivate(context);
  }

  handleRequest(err, user, info) {
      // console.log(user);
      if (err || !user) {
          throw err || new UnauthorizedException();
      }
      return user;
  }
}