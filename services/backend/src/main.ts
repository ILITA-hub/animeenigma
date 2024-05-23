import { NestFactory } from '@nestjs/core';
import { AppModule } from './app.module';
import { ValidationPipe } from '@nestjs/common';
import { Logger } from '@nestjs/common';
import { SwaggerModule, DocumentBuilder } from '@nestjs/swagger';
import { config } from './config/index'

import * as morgan from 'morgan';

async function bootstrap() {
  const app = await NestFactory.create(AppModule, {cors: true});
  app.useGlobalPipes(new ValidationPipe());
  app.use(morgan('dev'));

  const swaggerConfig = new DocumentBuilder()
    .setTitle('Anime Enigma API')
    .setDescription('The Anime Enigma API')
    .setVersion('1.0')
    .addServer("/")
    .addServer("/api")
    .addBearerAuth()
    .build();
  const document = SwaggerModule.createDocument(app, swaggerConfig);
  SwaggerModule.setup('doc', app, document);
  
  await app.listen(config.restPort);

  const logger = app.get(Logger);
  logger.log(`Application listening at ${await app.getUrl()}`);
}
bootstrap();
