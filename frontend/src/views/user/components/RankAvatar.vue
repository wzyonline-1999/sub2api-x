<template>
  <span :class="['rank-avatar', tone, { large, compact }]">
    <img
      v-if="resolvedAvatarUrl && !imageFailed"
      :src="resolvedAvatarUrl"
      alt=""
      :loading="compact ? 'lazy' : 'eager'"
      decoding="async"
      @error="imageFailed = true"
    />
    <span v-else class="rank-avatar-initial" aria-hidden="true">{{ fallback }}</span>
  </span>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'

type RankAvatarTone = 'gold' | 'silver' | 'bronze' | 'neutral'

const props = withDefaults(defineProps<{
  name: string
  avatarUrl?: string
  tone: RankAvatarTone
  large?: boolean
  compact?: boolean
}>(), {
  avatarUrl: '',
  large: false,
  compact: false,
})

const imageFailed = ref(false)
const resolvedAvatarUrl = computed(() => props.avatarUrl.trim())
const fallback = computed(() => props.name.trim().charAt(0).toUpperCase() || 'U')

watch(() => props.avatarUrl, () => {
  imageFailed.value = false
})
</script>

<style scoped>
.rank-avatar {
  display: inline-flex;
  width: 3.625rem;
  height: 3.625rem;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  overflow: hidden;
  border-radius: 50%;
  font-size: 1.25rem;
  font-weight: 900;
}

.rank-avatar.large {
  width: 4.75rem;
  height: 4.75rem;
  font-size: 1.7rem;
}

.rank-avatar.compact {
  width: 2.75rem;
  height: 2.75rem;
  font-size: 0.9375rem;
}

.rank-avatar.gold {
  border: 2px solid #f59e0b;
  background: #fef3c7;
  color: #b45309;
}

.rank-avatar.silver {
  border: 2px solid #94a3b8;
  background: #f1f5f9;
  color: #475569;
}

.rank-avatar.bronze {
  border: 2px solid #c2410c;
  background: #ffedd5;
  color: #9a3412;
}

.rank-avatar.neutral {
  border: 1px solid #d6e2e6;
  background: #eef6f6;
  color: #0f766e;
  box-shadow: 0 0 0 3px #f8fbfb;
}

img {
  display: block;
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.rank-avatar-initial {
  line-height: 1;
}

:global(.dark) .rank-avatar.gold {
  background: rgb(245 158 11 / 0.18);
  color: #facc15;
}

:global(.dark) .rank-avatar.silver {
  background: rgb(148 163 184 / 0.16);
  color: #cbd5e1;
}

:global(.dark) .rank-avatar.bronze {
  background: rgb(194 65 12 / 0.18);
  color: #fdba74;
}

:global(.dark) .rank-avatar.neutral {
  border-color: #3a4a5f;
  background: #243044;
  color: #5eead4;
  box-shadow: 0 0 0 3px #111827;
}

@media (max-width: 960px) {
  .rank-avatar.compact {
    width: 2.5rem;
    height: 2.5rem;
  }
}

@media (max-width: 480px) {
  .rank-avatar.large {
    width: 3.625rem;
    height: 3.625rem;
    font-size: 1.25rem;
  }
}
</style>
