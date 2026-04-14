<script setup lang="ts">
import { NConfigProvider, NMessageProvider, darkTheme } from 'naive-ui'
import { RouterView } from 'vue-router'
import AppShell from './layouts/AppShell.vue'
import { themeOverrides } from './theme'
</script>

<template>
  <n-config-provider :theme="darkTheme" :theme-overrides="themeOverrides">
    <n-message-provider>
      <router-view v-slot="{ Component, route }">
        <AppShell v-if="!route.meta.public">
          <component :is="Component" v-if="Component" />
        </AppShell>
        <component :is="Component" v-else-if="Component" />
      </router-view>
    </n-message-provider>
  </n-config-provider>
</template>

<style>
:root {
  color-scheme: dark;
  font-family: "Inter", "Segoe UI", "PingFang SC", sans-serif;
  --app-font-mono: "Fira Code", "SFMono-Regular", "Consolas", monospace;
  --app-bg: #0a101c;
  --app-bg-elevated: #0f172a;
  --app-surface: rgba(11, 18, 32, 0.96);
  --app-surface-muted: rgba(15, 23, 42, 0.82);
  --app-surface-strong: rgba(22, 32, 51, 0.94);
  --app-border: rgba(148, 163, 184, 0.14);
  --app-border-strong: rgba(148, 163, 184, 0.22);
  --app-text: #e5edf7;
  --app-text-soft: #9fb0c7;
  --app-text-faint: #6b7a90;
  --app-accent: #34d399;
  --app-accent-strong: #10b981;
  --app-danger: #fb7185;
  --app-warning: #fbbf24;
  --app-radius-sm: 8px;
  --app-radius-md: 12px;
  --app-radius-lg: 16px;
  --app-shadow-soft: 0 10px 24px rgba(2, 6, 23, 0.18);
}

* {
  box-sizing: border-box;
}

html,
body,
#app {
  margin: 0;
  min-height: 100%;
}

body {
  background-color: #050505;
  background-image: 
    radial-gradient(circle at 15% 50%, rgba(16, 185, 129, 0.1), transparent 25%),
    radial-gradient(circle at 85% 30%, rgba(59, 130, 246, 0.1), transparent 25%),
    linear-gradient(rgba(255, 255, 255, 0.03) 1px, transparent 1px),
    linear-gradient(90deg, rgba(255, 255, 255, 0.03) 1px, transparent 1px);
  background-size: 100% 100%, 100% 100%, 32px 32px, 32px 32px;
  background-position: 0 0, 0 0, 0 0, 0 0;
  color: var(--app-text);
}

a {
  color: inherit;
}

/* Custom Scrollbar for Webkit */
::-webkit-scrollbar {
  width: 8px;
  height: 8px;
}
::-webkit-scrollbar-track {
  background: transparent;
}
::-webkit-scrollbar-thumb {
  background: rgba(255, 255, 255, 0.15);
  border-radius: 4px;
}
::-webkit-scrollbar-thumb:hover {
  background: rgba(255, 255, 255, 0.3);
}

/* Page Transitions */
.fade-slide-enter-active,
.fade-slide-leave-active {
  transition: opacity 0.3s ease, transform 0.3s ease;
}
.fade-slide-enter-from {
  opacity: 0;
  transform: translateY(10px);
}
.fade-slide-leave-to {
  opacity: 0;
  transform: translateY(-10px);
}

.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.3s ease;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
