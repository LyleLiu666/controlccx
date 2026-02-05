<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";

type WorkdirOption = {
  value: string;
  label: string;
  subLabel?: string;
};

const props = withDefaults(
  defineProps<{
    modelValue: string;
    pinned: WorkdirOption[];
    recent: WorkdirOption[];
    placeholder?: string;
    disabled?: boolean;
  }>(),
  {
    pinned: () => [],
    recent: () => [],
    placeholder: ".",
    disabled: false,
  },
);

const emit = defineEmits<{
  (e: "update:modelValue", value: string): void;
}>();

const rootEl = ref<HTMLElement | null>(null);
const inputEl = ref<HTMLInputElement | null>(null);
const menuEl = ref<HTMLElement | null>(null);
const open = ref(false);
const activeIndex = ref(-1);

const menuId =
  typeof crypto !== "undefined" && "randomUUID" in crypto
    ? `workdir-menu-${crypto.randomUUID()}`
    : `workdir-menu-${Math.random().toString(16).slice(2)}`;

const flatOptions = computed<WorkdirOption[]>(() => [
  ...(props.pinned ?? []),
  ...(props.recent ?? []),
]);

const hasOptions = computed(() => flatOptions.value.length > 0);

function clampIndex(idx: number): number {
  const max = flatOptions.value.length - 1;
  if (max < 0) return -1;
  return Math.max(0, Math.min(max, idx));
}

function ensureActiveIndex() {
  if (!hasOptions.value) {
    activeIndex.value = -1;
    return;
  }
  if (activeIndex.value < 0 || activeIndex.value >= flatOptions.value.length) {
    const current = (props.modelValue ?? "").trim();
    const match = flatOptions.value.findIndex((o) => o.value === current);
    activeIndex.value = match >= 0 ? match : 0;
  }
}

function scrollActiveIntoView() {
  const menu = menuEl.value;
  if (!menu) return;
  const idx = activeIndex.value;
  if (idx < 0) return;
  const el = menu.querySelector(`[data-opt-idx="${idx}"]`) as HTMLElement | null;
  el?.scrollIntoView({ block: "nearest" });
}

function openMenu() {
  if (props.disabled) return;
  if (open.value) return;
  open.value = true;
  ensureActiveIndex();
  void nextTick(() => scrollActiveIntoView());
}

function closeMenu() {
  open.value = false;
  activeIndex.value = -1;
}

function toggleMenu() {
  if (open.value) closeMenu();
  else openMenu();
}

function selectValue(value: string) {
  emit("update:modelValue", value);
  closeMenu();
  void nextTick(() => inputEl.value?.focus());
}

function onToggleMouseDown(e: MouseEvent) {
  e.preventDefault(); // keep focus on input
  inputEl.value?.focus();
}

function onToggleClick(e: MouseEvent) {
  // Keyboard activation (Enter/Space) won’t trigger mousedown.
  e.preventDefault();
  toggleMenu();
  inputEl.value?.focus();
}

function onInput(e: Event) {
  emit("update:modelValue", (e.target as HTMLInputElement).value);
}

function moveActive(delta: number) {
  if (!hasOptions.value) return;
  ensureActiveIndex();
  activeIndex.value = clampIndex(activeIndex.value + delta);
  scrollActiveIntoView();
}

function onKeyDown(e: KeyboardEvent) {
  if (e.key === "ArrowDown") {
    e.preventDefault();
    if (!open.value) openMenu();
    else moveActive(1);
    return;
  }
  if (e.key === "ArrowUp") {
    e.preventDefault();
    if (!open.value) openMenu();
    else moveActive(-1);
    return;
  }
  if (e.key === "Escape") {
    if (!open.value) return;
    e.preventDefault();
    closeMenu();
    return;
  }
  if (e.key === "Tab") {
    if (!open.value) return;
    closeMenu();
    return;
  }
  if (e.key === "Enter") {
    if (!open.value) return;
    if (activeIndex.value < 0 || activeIndex.value >= flatOptions.value.length) return;
    e.preventDefault();
    selectValue(flatOptions.value[activeIndex.value]!.value);
  }
}

function onDocumentMouseDown(e: MouseEvent) {
  if (!open.value) return;
  const target = e.target as Node | null;
  if (!target) return;
  const root = rootEl.value;
  if (root && root.contains(target)) return;
  closeMenu();
}

watch(
  () => props.disabled,
  (disabled) => {
    if (disabled) closeMenu();
  },
);

watch(flatOptions, () => {
  if (!open.value) return;
  ensureActiveIndex();
  void nextTick(() => scrollActiveIntoView());
});

onMounted(() => {
  document.addEventListener("mousedown", onDocumentMouseDown);
});

onBeforeUnmount(() => {
  document.removeEventListener("mousedown", onDocumentMouseDown);
});
</script>

<template>
  <div ref="rootEl" class="workdirCombo" :class="{ open }">
    <div class="workdirComboInputWrap">
      <input
        ref="inputEl"
        class="workdirComboInput"
        :value="modelValue"
        :placeholder="placeholder"
        :disabled="disabled"
        autocomplete="off"
        autocapitalize="off"
        spellcheck="false"
        :aria-expanded="open ? 'true' : 'false'"
        :aria-controls="menuId"
        @input="onInput"
        @keydown="onKeyDown"
      />

      <button
        type="button"
        class="workdirComboToggle"
        :disabled="disabled"
        aria-label="工作目录建议"
        aria-haspopup="listbox"
        :aria-expanded="open ? 'true' : 'false'"
        :aria-controls="menuId"
        @mousedown="onToggleMouseDown"
        @click="onToggleClick"
      >
        <svg
          class="chev"
          viewBox="0 0 20 20"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
          aria-hidden="true"
        >
          <path d="M6 8l4 4 4-4" />
        </svg>
      </button>
    </div>

    <div
      v-if="open"
      :id="menuId"
      ref="menuEl"
      class="workdirComboMenu"
      role="listbox"
    >
      <div v-if="!hasOptions" class="workdirComboEmpty">
        暂无历史工作目录
      </div>

      <template v-else>
        <div v-if="pinned.length" class="workdirComboGroup">
          <div class="workdirComboGroupLabel">Pinned</div>
          <button
            v-for="(o, i) in pinned"
            :key="'p-' + o.value"
            type="button"
            class="workdirComboItem"
            role="option"
            :title="o.value"
            :aria-selected="activeIndex === i ? 'true' : 'false'"
            :class="{ active: activeIndex === i }"
            :data-opt-idx="i"
            @mousedown.prevent="selectValue(o.value)"
          >
            <span class="workdirComboItemMain">{{ o.label }}</span>
            <span v-if="o.subLabel" class="workdirComboItemSub mono">{{
              o.subLabel
            }}</span>
          </button>
        </div>

        <div v-if="recent.length" class="workdirComboGroup">
          <div class="workdirComboGroupLabel">Recent</div>
          <button
            v-for="(o, i) in recent"
            :key="'r-' + o.value"
            type="button"
            class="workdirComboItem"
            role="option"
            :title="o.value"
            :aria-selected="activeIndex === pinned.length + i ? 'true' : 'false'"
            :class="{ active: activeIndex === pinned.length + i }"
            :data-opt-idx="pinned.length + i"
            @mousedown.prevent="selectValue(o.value)"
          >
            <span class="workdirComboItemMain">{{ o.label }}</span>
            <span v-if="o.subLabel" class="workdirComboItemSub mono">{{
              o.subLabel
            }}</span>
          </button>
        </div>
      </template>
    </div>
  </div>
</template>

<style scoped>
.workdirCombo {
  position: relative;
  width: 100%;
  min-width: 0;
}

.workdirComboInputWrap {
  position: relative;
  width: 100%;
}

.workdirComboInput {
  padding-right: 46px;
}

.workdirComboToggle {
  position: absolute;
  top: 50%;
  right: 8px;
  transform: translateY(-50%);
  width: 36px;
  height: 36px;
  padding: 0;
  border-radius: 12px;
  display: grid;
  place-items: center;
  border: 1px solid transparent;
  background: transparent;
  color: var(--text-sub);
}

.workdirComboToggle:hover:not(:disabled) {
  background: var(--bg-panel);
  border-color: rgba(148, 163, 184, 0.35);
  color: var(--text-main);
  box-shadow: var(--shadow-sm);
}

.workdirComboToggle:focus-visible {
  outline: 2px solid rgba(45, 212, 191, 0.55);
  outline-offset: 2px;
}

.chev {
  width: 18px;
  height: 18px;
  transition: transform 0.18s ease;
}

.workdirCombo.open .chev {
  transform: rotate(180deg);
}

.workdirCombo.open .workdirComboToggle {
  background: var(--bg-panel);
  border-color: rgba(148, 163, 184, 0.28);
  color: var(--text-main);
}

.workdirComboMenu {
  position: absolute;
  left: 0;
  right: 0;
  top: calc(100% + 8px);
  z-index: 40;
  background-image: linear-gradient(180deg, var(--bg-panel) 0%, var(--bg-subtle) 100%);
  border: 1px solid var(--border-color);
  border-radius: 14px;
  box-shadow: var(--shadow-lg);
  padding: 8px;
  max-height: min(320px, 40vh);
  overflow: auto;
  backdrop-filter: blur(12px);
}

.workdirComboEmpty {
  padding: 10px 10px;
  color: var(--text-sub);
  font-size: 13px;
}

.workdirComboGroup {
  display: grid;
  gap: 6px;
}

.workdirComboGroup + .workdirComboGroup {
  margin-top: 10px;
  padding-top: 10px;
  border-top: 1px solid var(--border-color);
}

.workdirComboGroupLabel {
  font-size: 11px;
  font-weight: 800;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--text-sub);
  padding: 2px 8px;
}

.workdirComboItem {
  width: 100%;
  text-align: left;
  padding: 10px 10px;
  min-height: 44px;
  border-radius: 12px;
  border: 1px solid transparent;
  background: transparent;
  display: grid;
  gap: 2px;
  cursor: pointer;
}

.workdirComboItem:hover {
  background: var(--color-primary-bg);
  border-color: var(--border-color);
}

.workdirComboItem.active {
  background: var(--color-primary-bg);
  border-color: var(--color-primary);
}

.workdirComboItem:focus-visible {
  outline: 2px solid rgba(45, 212, 191, 0.55);
  outline-offset: 2px;
}

.workdirComboItemMain {
  font-size: 13px;
  font-weight: 650;
  color: var(--text-main);
  line-height: 1.2;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.workdirComboItemSub {
  font-size: 12px;
  color: var(--text-sub);
  opacity: 0.95;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
