import { Module, OnModuleInit } from '@nestjs/common';
import { TypeOrmModule } from '@nestjs/typeorm';
import { MulterModule } from '@nestjs/platform-express';
import { StreamingController } from './streaming.controller';
import { StreamingService } from './streaming.service';
import { StreamingSource } from './entity/streaming-source.entity';
import { AnimeSource } from './entity/anime-source.entity';
import { AnimeEntity } from '../anime/entity/anime.entity';
import { AuthModule } from '../auth/auth.module';

@Module({
  imports: [
    TypeOrmModule.forFeature([StreamingSource, AnimeSource, AnimeEntity]),
    MulterModule.register({
      limits: {
        fileSize: 500 * 1024 * 1024, // 500MB max file size
      },
    }),
    AuthModule,
  ],
  controllers: [StreamingController],
  providers: [StreamingService],
  exports: [StreamingService],
})
export class StreamingModule implements OnModuleInit {
  constructor(private readonly streamingService: StreamingService) {}

  async onModuleInit() {
    // Initialize default streaming sources on startup
    await this.streamingService.initializeDefaultSources();
  }
}
