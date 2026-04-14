<script setup lang="ts">
defineProps<{
  eyebrow?: string
  title: string
  description?: string
}>()
</script>

<template>
  <header class="page-header">
    <div class="page-header__copy">
      <p v-if="eyebrow" class="page-header__eyebrow">{{ eyebrow }}</p>
      <div class="page-header__title-row">
        <h2>{{ title }}</h2>
        <div v-if="$slots.meta" class="page-header__meta">
          <slot name="meta" />
        </div>
      </div>
      <p v-if="description" class="page-header__description">{{ description }}</p>
    </div>

    <div v-if="$slots.actions" class="page-header__actions">
      <slot name="actions" />
    </div>
  </header>
</template>

<style scoped>
.page-header {
  display: flex;
  justify-content: space-between;
  gap: 18px;
  align-items: flex-start;
  padding: 0 0 14px;
  border-bottom: 1px solid var(--app-border);
}

.page-header__copy {
  min-width: 0;
  display: grid;
  gap: 8px;
}

.page-header__eyebrow {
  margin: 0;
  color: var(--app-accent);
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.14em;
  text-transform: uppercase;
}

.page-header__title-row {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.page-header__title-row h2 {
  margin: 0;
  font-size: clamp(22px, 2.1vw, 28px);
  line-height: 1.1;
  color: #f8fafc;
  letter-spacing: -0.02em;
}

.page-header__meta {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.page-header__description {
  margin: 0;
  max-width: 760px;
  color: var(--app-text-soft);
  line-height: 1.65;
  font-size: 13px;
}

.page-header__actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 10px;
  flex-wrap: wrap;
  flex: none;
}

@media (max-width: 900px) {
  .page-header {
    flex-direction: column;
    align-items: flex-start;
  }

  .page-header__actions {
    justify-content: flex-start;
  }
}
</style>
