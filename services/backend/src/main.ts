import { NestFactory } from '@nestjs/core';
import { AppModule } from './app.module';
import { ValidationPipe } from '@nestjs/common';
import { Logger } from '@nestjs/common';
import { SwaggerModule, DocumentBuilder, SwaggerCustomOptions } from '@nestjs/swagger';
import { NextFunction, Request, Response } from 'express';

import * as morgan from 'morgan';

async function bootstrap() {
  const app = await NestFactory.create(AppModule);
  app.use(function (request: Request, response: Response, next: NextFunction) {
    response.setHeader('Access-Control-Allow-Origin', '*');
    next();
  });
  app.useGlobalPipes(new ValidationPipe());
  app.use(morgan('dev'));

  const config = new DocumentBuilder()
    .setTitle('Anime Enigma API')
    .setDescription('The Anime Enigma API')
    .setVersion('1.0')
    .addServer("/")
    .addServer("/api")
    .addBearerAuth()
    .build();
  const document = SwaggerModule.createDocument(app, config);
  SwaggerModule.setup('doc', app, document);
  
  await app.listen(3000);

  const logger = app.get(Logger);
  logger.log(`Application listening at ${await app.getUrl()}`);
}
bootstrap();
