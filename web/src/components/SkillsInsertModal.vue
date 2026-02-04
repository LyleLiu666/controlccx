<script setup lang="ts">
import { computed, nextTick, ref, watch } from "vue";
import { fetchSkills } from "../api";
import { summarizeSkillTarget, type SkillTarget, type SkillsSummary } from "../skillsSummary";
import { formatSkillToken } from "../skillsInvoke";
import type { Skill, ToolDriver } from "../types";

const props = defineProps<{
  open: boolean;
  driver: ToolDriver;
  prompt: string;
  promptEl: HTMLTextAreaElement | HTMLInputElement | null;
}>();

const emit = defineEmits<{
  (e: "close"): void;
  (e: "update:prompt", val: string): void;
}>();

const canUseSkills = computed<boolean>(() => props.driver === "claude-code" || props.driver === "codex");

const skillsPickerOpen = computed<boolean>(() => props.open && canUseSkills.value);
const skillsQuery = ref("");
const skillsOnlyAvailable = ref(true);
const skillsLoading = ref(false);
const skillsError = ref("");
const skillsData = ref<Skill[]>([]);
const skillsSearchEl = ref<HTMLInputElement | null>(null);

const skillsTarget = computed<SkillTarget | null>(() => {
  switch (props.driver) {
    case "claude-code":
      return "claude_code";
    case "codex":
      return "codex";
    default:
      return null;
  }
});

function skillStatusLabel(status: SkillsSummary["status"]): string {
  switch (status) {
    case "missing":
      return "缺失";
    case "linked":
      return "已关联";
    case "copied":
      return "已复制";
    case "present":
      return "已存在";
    case "external":
      return "外部";
    case "broken":
      return "异常";
    case "conflict":
      return "冲突";
    case "partial":
      return "部分";
    default:
      return String(status).toUpperCase();
  }
}

function skillStatusBadgeClass(status: SkillsSummary["status"]): string {
  switch (status) {
    case "linked":
    case "copied":
      return "ok";
    case "present":
    case "external":
      return "muted";
    case "partial":
      return "partial";
    case "missing":
      return "dim";
    default:
      return "warn";
  }
}

function isSkillAvailable(summary: SkillsSummary): boolean {
  return summary.status !== "missing" && summary.status !== "broken";
}

const skillsVisible = computed(() => {
  const q = skillsQuery.value.trim().toLowerCase();
  const target = skillsTarget.value;

  const out: Array<{ name: string; summary: SkillsSummary | null; available: boolean; title: string }> = [];
  for (const s of skillsData.value) {
    if (q && !String(s.name ?? "").toLowerCase().includes(q)) continue;

    const summary = target ? summarizeSkillTarget(s, target) : null;
    const available = summary ? isSkillAvailable(summary) : false;
    if (skillsOnlyAvailable.value && !available) continue;
    out.push({
      name: s.name,
      summary,
      available,
      title: summary?.detail ?? "",
    });
  }

  out.sort((a, b) => a.name.localeCompare(b.name));
  return out.slice(0, 200);
});

async function ensureSkillsLoaded(force = false) {
  if (!canUseSkills.value) return;
  if (skillsLoading.value) return;
  if (!force && skillsData.value.length) return;
  skillsLoading.value = true;
  skillsError.value = "";
  try {
    const res = await fetchSkills({ limit: 500, offset: 0 });
    const list = Array.isArray(res.skills) ? res.skills.slice() : [];
    list.sort((a, b) => String(a.name).localeCompare(String(b.name)));
    skillsData.value = list;
  } catch (e: any) {
    skillsError.value = e?.message ?? String(e);
  } finally {
    skillsLoading.value = false;
  }
}

function closeSkillsPicker() {
  const promptEl = props.promptEl;
  emit("close");
  void nextTick(() => {
    try {
      promptEl?.focus();
    } catch {
      // ignore
    }
  });
}

function onSkillsPickerKeyDown(ev: KeyboardEvent) {
  if (!skillsPickerOpen.value) return;
  if (ev.key !== "Escape") return;
  ev.preventDefault();
  closeSkillsPicker();
}

function insertIntoPrompt(text: string) {
  const current = String(props.prompt ?? "");
  const el = props.promptEl;
  const start = el?.selectionStart ?? current.length;
  const end = el?.selectionEnd ?? start;
  const next = current.slice(0, start) + text + current.slice(end);
  emit("update:prompt", next);
  void nextTick(() => {
    const nextPos = start + text.length;
    try {
      el?.focus();
      el?.setSelectionRange(nextPos, nextPos);
    } catch {
      // ignore
    }
  });
}

function chooseSkill(name: string) {
  if (!canUseSkills.value) return;
  const token = formatSkillToken(name, props.driver);
  if (!token) return;

  const insertText = token.startsWith("/") ? token + "\n" : token + " ";
  insertIntoPrompt(insertText);
  closeSkillsPicker();
}

watch(
  () => skillsPickerOpen.value,
  async (open) => {
    if (!open) {
      skillsError.value = "";
      skillsQuery.value = "";
      return;
    }
    skillsError.value = "";
    void ensureSkillsLoaded();
    await nextTick();
    skillsSearchEl.value?.focus();
  }
);
</script>

<template>
  <div
    v-if="skillsPickerOpen"
    class="modalOverlay newRunSkillsOverlay"
    @click.self="closeSkillsPicker"
    @keydown.capture="onSkillsPickerKeyDown"
  >
    <div class="modal smallModal newRunSkillsModal" role="dialog" aria-modal="true">
      <div class="modalHeader">
        <div class="modalTitle">选择技能</div>
        <button class="iconBtn" type="button" @click="closeSkillsPicker">✕</button>
      </div>
      <div class="modalBody newRunSkillsBody">
        <div class="tinyHint">
          当前工具：<span class="mono">{{ driver }}</span> · 插入：
          <span class="mono">{{ formatSkillToken("my-skill", driver) }}</span>
        </div>

        <div class="newRunSkillsToolbar">
          <input
            ref="skillsSearchEl"
            v-model="skillsQuery"
            placeholder="搜索技能（名称）…"
            :disabled="skillsLoading"
          />
          <label class="newRunSkillsOnly">
            <input type="checkbox" v-model="skillsOnlyAvailable" />
            <span>仅显示可用</span>
          </label>
          <button
            type="button"
            @click="ensureSkillsLoaded(true)"
            :disabled="skillsLoading"
            title="刷新技能列表"
          >
            刷新
          </button>
        </div>

        <div v-if="skillsError" class="modalError">{{ skillsError }}</div>
        <div v-else-if="skillsLoading" class="loading">加载中…</div>
        <div v-else class="newRunSkillsList" role="listbox" aria-label="Skills">
          <button
            v-for="s in skillsVisible"
            :key="s.name"
            type="button"
            class="newRunSkillOption"
            :disabled="!s.available"
            :title="s.title"
            @click="chooseSkill(s.name)"
          >
            <span class="mono">{{ s.name }}</span>
            <span
              v-if="s.summary"
              class="pill mono skillStatus"
              :class="skillStatusBadgeClass(s.summary.status)"
              >{{ skillStatusLabel(s.summary.status) }}</span
            >
            <span v-else class="pill mono skillStatus dim">未知</span>
          </button>
          <div v-if="!skillsVisible.length" class="empty">暂无匹配技能</div>
        </div>
      </div>
      <div class="modalFooter">
        <button type="button" @click="closeSkillsPicker">关闭</button>
      </div>
    </div>
  </div>
</template>

