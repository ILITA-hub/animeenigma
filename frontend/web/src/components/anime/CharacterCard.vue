<template>
  <router-link :to="`/characters/${model.id}`" class="group block" :aria-label="model.name">
    <div class="rounded-xl overflow-hidden bg-white/5 border border-white/10 transition-[border-color,box-shadow] duration-200 group-hover:border-white/20 group-hover:shadow-[0_10px_30px_rgba(0,0,0,0.4)]">
      <div class="relative aspect-[2/3] bg-white/5">
        <img
          :src="model.image"
          :alt="model.name"
          loading="lazy"
          class="absolute inset-0 w-full h-full object-cover"
        />
        <div class="absolute top-2 left-2">
          <Badge
            data-testid="role-badge"
            :variant="model.role === 'main' ? 'primary' : 'default'"
            size="sm"
            :overlay="true"
          >
            {{ model.role === 'main' ? t('characters.main') : t('characters.supporting') }}
          </Badge>
        </div>
      </div>
      <div class="px-2.5 pt-2.5 pb-3">
        <h3 class="font-medium text-white line-clamp-2 text-[13px] leading-[1.3] min-h-[2.6em] group-hover:text-cyan-400 transition-colors">
          {{ model.name }}
        </h3>
      </div>
    </div>
  </router-link>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Badge from '@/components/ui/Badge.vue'
import type { CharacterCardModel } from '@/types/character'

defineProps<{ model: CharacterCardModel }>()

const { t } = useI18n()
</script>
