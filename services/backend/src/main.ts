import { NestFactory } from '@nestjs/core';
import { AppModule } from './app.module';
import { ValidationPipe } from '@nestjs/common';
import { Logger } from '@nestjs/common';

import * as morgan from 'morgan';

async function bootstrap() {
  const app = await NestFactory.create(AppModule);
  app.useGlobalPipes(new ValidationPipe());
  app.use(morgan('dev'));
  
  await app.listen(3000);

  const logger = app.get(Logger);
  logger.log(`Application listening at ${await app.getUrl()}`);
}
bootstrap();
