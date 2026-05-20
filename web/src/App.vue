<script setup lang="ts">
import { NConfigProvider, NDialogProvider, NMessageProvider, darkTheme, zhCN, dateZhCN } from 'naive-ui'
import { RouterView } from 'vue-router'
import AppShell from './layouts/AppShell.vue'
import { getThemeOverrides } from './theme'
</script>

<template>
  <n-config-provider :theme="darkTheme" :theme-overrides="getThemeOverrides('dark')" :locale="zhCN" :date-locale="dateZhCN">
    <n-dialog-provider>
      <n-message-provider>
        <router-view v-slot="{ Component, route }">
          <AppShell v-if="!route.meta.public">
            <component :is="Component" v-if="Component" />
          </AppShell>
          <component :is="Component" v-else-if="Component" />
        </router-view>
      </n-message-provider>
    </n-dialog-provider>
  </n-config-provider>
</template>

<style>
:root {
  color-scheme: dark;
  font-family: "Inter", "Segoe UI", "PingFang SC", sans-serif;
  --app-font-mono: "Fira Code", "SFMono-Regular", "Consolas", monospace;
  --app-bg: #08111f;
  --app-bg-elevated: #0d1829;
  --app-surface: rgba(13, 24, 41, 0.86);
  --app-surface-muted: rgba(17, 31, 51, 0.72);
  --app-surface-strong: rgba(21, 37, 61, 0.86);
  --app-panel: linear-gradient(180deg, rgba(14, 28, 48, 0.74), rgba(8, 18, 32, 0.84));
  --app-panel-flat: rgba(12, 25, 43, 0.72);
  --app-panel-strong: rgba(12, 25, 43, 0.9);
  --app-panel-soft: rgba(17, 32, 52, 0.5);
  --app-control: rgba(12, 24, 42, 0.72);
  --app-control-strong: rgba(16, 31, 53, 0.9);
  --app-table-row: rgba(12, 24, 42, 0.34);
  --app-table-row-hover: rgba(79, 131, 255, 0.08);
  --app-border: rgba(93, 120, 162, 0.18);
  --app-border-strong: rgba(109, 139, 188, 0.3);
  --app-text: #f3f7ff;
  --app-text-soft: #aab8cd;
  --app-text-faint: #6f8098;
  --app-accent: #20d4ff;
  --app-accent-strong: #4f83ff;
  --app-danger: #ff6b7d;
  --app-warning: #e8b45f;
  --app-info: #20d4ff;
  --app-radius-sm: 8px;
  --app-radius-md: 8px;
  --app-radius-lg: 8px;
  --app-shadow-soft: 0 10px 24px rgba(0, 8, 22, 0.28);
  --app-background-image: url('/hostdeck-background.png');
  --app-background-overlay:
    radial-gradient(circle at 78% 8%, rgba(32, 212, 255, 0.06), transparent 24%),
    linear-gradient(90deg, rgba(2, 8, 17, 0.04), rgba(2, 8, 17, 0.14) 58%, rgba(2, 8, 17, 0.32)),
    linear-gradient(180deg, rgba(8, 17, 31, 0.34), rgba(4, 9, 17, 0.56));
  --app-background-size: cover;
  --app-background-position: center top;
  --app-background-repeat: no-repeat;
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

html {
  -webkit-text-size-adjust: 100%;
  text-size-adjust: 100%;
}

button,
input,
textarea,
select {
  font: inherit;
  letter-spacing: 0;
}

body {
  background-color: var(--app-bg);
  background-image: var(--app-background-overlay), var(--app-background-image);
  background-size: var(--app-background-size);
  background-position: var(--app-background-position);
  background-repeat: var(--app-background-repeat);
  background-attachment: fixed;
  color: var(--app-text);
}

a {
  color: inherit;
}

body .page-container {
  width: 100%;
  max-width: 100%;
  margin: 0;
  padding: 18px 20px 24px;
}

body .page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 16px;
  margin-bottom: 18px;
}

body .header-text h1,
body .title-row h1 {
  margin: 0;
  color: var(--app-text);
  font-size: 22px;
  font-weight: 700;
  letter-spacing: 0;
  line-height: 1.2;
}

body .header-text p {
  display: none;
}

body .header-actions,
body .action-stack,
body .inline-actions,
body .modal-actions {
  gap: 8px;
}

body .glow-btn {
  min-height: 34px;
  border-radius: 8px !important;
  box-shadow: none !important;
  background-color: var(--app-accent-strong) !important;
  color: #fff !important;
  transform: none !important;
}

body .glow-btn:hover:not(:disabled) {
  box-shadow: none !important;
  background-color: #356fe8 !important;
  transform: none !important;
}

body .n-button {
  border-radius: 8px;
}

body .n-button--primary-type {
  box-shadow: none !important;
}

body .n-button--primary-type:not(.n-button--disabled):not(.n-button--ghost):not(.n-button--text):not(.n-button--quaternary):not(.n-button--tertiary) {
  background-color: var(--app-accent-strong) !important;
  border-color: var(--app-accent-strong) !important;
}

body .n-button--primary-type:hover:not(.n-button--disabled):not(.n-button--ghost):not(.n-button--text):not(.n-button--quaternary):not(.n-button--tertiary) {
  background-color: #2f72ff !important;
  border-color: #2f72ff !important;
}

body .n-button--error-type:not(.n-button--disabled):not(.n-button--ghost):not(.n-button--text):not(.n-button--quaternary):not(.n-button--tertiary) {
  color: var(--app-danger) !important;
  border-color: rgba(255, 107, 125, 0.45) !important;
}

body .n-tag {
  border-radius: 6px !important;
}

body .n-tabs .n-tabs-tab {
  color: var(--app-text-soft);
  font-weight: 600;
}

body .n-tabs .n-tabs-tab--active {
  color: #62a8ff;
}

body .n-tabs .n-tabs-bar {
  background-color: #2f81ff !important;
}

body .dark-input .n-input-wrapper,
body .dark-input .n-base-selection,
body .dark-input .n-input-number {
  min-height: 40px;
  border-radius: 8px !important;
  background: rgba(9, 20, 36, 0.88) !important;
  background-color: rgba(9, 20, 36, 0.88) !important;
  border: 1px solid rgba(93, 120, 162, 0.24) !important;
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.04) !important;
}

body .dark-input {
  color-scheme: dark;
}

body .dark-input.n-input,
body .dark-input.n-base-selection,
body .dark-input.n-input-number,
body .dark-input.n-date-picker {
  --n-color: rgba(9, 20, 36, 0.88) !important;
  --n-color-focus: rgba(12, 27, 48, 0.96) !important;
  --n-color-disabled: rgba(10, 18, 31, 0.68) !important;
  --n-border: 1px solid rgba(93, 120, 162, 0.24) !important;
  --n-border-hover: 1px solid rgba(79, 131, 255, 0.42) !important;
  --n-border-focus: 1px solid rgba(79, 131, 255, 0.66) !important;
  --n-box-shadow-focus: 0 0 0 2px rgba(79, 131, 255, 0.12) !important;
  --n-text-color: var(--app-text) !important;
  --n-placeholder-color: var(--app-text-faint) !important;
  --n-caret-color: var(--app-accent) !important;
}

body .dark-input .n-input-wrapper:hover,
body .dark-input .n-base-selection:hover,
body .dark-input .n-input-number:hover {
  border-color: rgba(79, 131, 255, 0.42) !important;
}

body .dark-input .n-input__border,
body .dark-input .n-input__state-border,
body .dark-input .n-base-selection__border,
body .dark-input .n-base-selection__state-border {
  display: none !important;
}

body .dark-input .n-input__input-el,
body .dark-input .n-input__textarea-el,
body .dark-input .n-input__placeholder,
body .dark-input .n-base-selection-input,
body .dark-input .n-base-selection-placeholder,
body .dark-input .n-date-picker-icon,
body .dark-input .n-date-picker-suffix {
  color: var(--app-text) !important;
}

body .dark-input .n-input__placeholder,
body .dark-input .n-base-selection-placeholder {
  color: var(--app-text-faint) !important;
}

body .dark-input .n-base-selection-label,
body .dark-input .n-base-selection-input,
body .dark-input .n-base-selection-tags,
body .dark-input .n-input-number-input,
body .dark-input .n-input-number-input__input,
body .dark-input .n-date-picker-rel,
body .dark-input .n-date-picker .n-input {
  color: var(--app-text) !important;
  background: transparent !important;
  background-color: transparent !important;
}

body .dark-input .n-input__input,
body .dark-input .n-input__textarea,
body .dark-input .n-input__input-el,
body .dark-input .n-input__textarea-el {
  background: transparent !important;
  background-color: transparent !important;
  color-scheme: dark !important;
}

body .dark-input .n-input__input-el,
body .dark-input .n-input__textarea-el,
body .dark-input .n-base-selection-input,
body .dark-input .n-input-number-input__input {
  font-size: 13px !important;
  line-height: 1.45 !important;
  letter-spacing: 0 !important;
}

body .dark-input .n-input__input-el::first-line,
body .dark-input .n-input__textarea-el::first-line,
body .dark-input .n-input-number-input__input::first-line {
  font-family: "Inter", "Segoe UI", "PingFang SC", sans-serif !important;
  font-size: 13px !important;
  line-height: 1.45 !important;
}

body .dark-input .n-input__input-el:-webkit-autofill,
body .dark-input .n-input__input-el:-webkit-autofill:hover,
body .dark-input .n-input__input-el:-webkit-autofill:focus,
body .dark-input .n-input__input-el:-webkit-autofill:active {
  caret-color: var(--app-accent) !important;
  -webkit-text-fill-color: var(--app-text) !important;
  background-color: rgba(9, 20, 36, 0.88) !important;
  box-shadow: 0 0 0 1000px rgba(9, 20, 36, 0.88) inset !important;
  transition: background-color 99999s ease-out, color 99999s ease-out !important;
}

body .dark-input .n-input__input-el:-webkit-autofill::first-line {
  color: var(--app-text) !important;
  font-family: "Inter", "Segoe UI", "PingFang SC", sans-serif !important;
  font-size: 13px !important;
}

body .compact-select .n-base-selection {
  min-height: 30px !important;
}

body .compact-select .n-base-selection-label {
  min-height: 28px !important;
}

body .dark-input .n-base-selection-tag {
  border: 1px solid rgba(93, 120, 162, 0.24) !important;
  border-radius: 6px !important;
  background: rgba(26, 42, 66, 0.9) !important;
  color: #dce7f6 !important;
}

body .dark-input .n-base-selection__clear,
body .dark-input .n-base-selection__arrow,
body .dark-input .n-input__suffix,
body .dark-input .n-input-number-suffix,
body .dark-input .n-input-number-button {
  color: var(--app-text-soft) !important;
}

body .n-base-select-menu,
body .n-date-panel,
body .n-popover,
body .n-dropdown-menu {
  border: 1px solid rgba(93, 120, 162, 0.24) !important;
  background: rgba(10, 19, 33, 0.98) !important;
  box-shadow: 0 18px 38px rgba(0, 0, 0, 0.36) !important;
}

body .n-base-select-option {
  color: #dce7f6 !important;
}

body .n-base-select-option.n-base-select-option--pending,
body .n-base-select-option.n-base-select-option--selected {
  background: rgba(79, 131, 255, 0.14) !important;
}

body .panel,
body .table-card,
body .control-panel,
body .results-panel-shell,
body .history-card,
body .content-card,
body .console-panel,
body .summary-panel,
body .info-panel,
body .trend-panel,
body .alert-board,
body .alert-side {
  border: 1px solid rgba(93, 120, 162, 0.22) !important;
  background: var(--app-panel) !important;
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.045), 0 14px 30px rgba(0, 8, 22, 0.18) !important;
}

body .dark-table,
body .dark-table .n-table,
body .dark-table .n-data-table-wrapper {
  background: transparent !important;
}

body .dark-table .n-data-table-th,
body .dark-table th {
  color: var(--app-text-soft) !important;
  background: rgba(17, 32, 52, 0.52) !important;
  border-color: rgba(93, 120, 162, 0.16) !important;
  font-size: 12px !important;
  font-weight: 600 !important;
}

body .dark-table .n-data-table-td,
body .dark-table td {
  color: #dce7f6 !important;
  background: rgba(7, 17, 31, 0.16) !important;
  border-color: rgba(93, 120, 162, 0.12) !important;
  font-size: 13px !important;
}

body .dark-table .n-data-table-tr:hover .n-data-table-td,
body .dark-table tr:hover td {
  background: var(--app-table-row-hover) !important;
}

body .shell-user-menu.n-dropdown-menu {
  padding: 6px !important;
  border: 1px solid rgba(93, 120, 162, 0.22) !important;
  border-radius: 8px !important;
  background: rgba(10, 19, 33, 0.98) !important;
  box-shadow: 0 18px 38px rgba(0, 0, 0, 0.36) !important;
}

body .shell-user-menu .n-dropdown-option {
  border-radius: 8px !important;
}

body .shell-user-menu .n-dropdown-option-body {
  color: #e8f0ff !important;
}

body .shell-user-menu .n-dropdown-option-body--pending {
  background: rgba(79, 131, 255, 0.12) !important;
}

body .n-card,
body .n-modal .n-card,
body .bento-card,
body .table-card,
body .control-panel,
body .history-card,
body .console-panel,
body .content-card,
body .resource-card,
body .result-card,
body .alert-content,
body .token-item,
body .role-matrix-item,
body .risk-banner,
body .info-item {
  border-radius: 8px !important;
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
