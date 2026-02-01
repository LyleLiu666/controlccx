<script setup lang="ts">
import { computed } from "vue";
import type { Skill, SkillsListResponse } from "../types";

type SkillTarget = "cursor" | "claude_code" | "codex";
type SkillsSummary = {
  target: SkillTarget;
  status:
    | "missing"
    | "linked"
    | "broken"
    | "present"
    | "copied"
    | "conflict"
    | "external"
    | "partial";
  canEnable: boolean;
  canDisable: boolean;
  enabled: boolean;
  detail: string;
};

const props = defineProps<{
  loading: boolean;
  error: string;
  data: SkillsListResponse | null;
  filter: string;
  limit: number;
  rangeLabel: string;
  canPrev: boolean;
  canNext: boolean;
  actionBusy: Map<string, boolean>;
  summarizeTarget: (skill: Skill, target: SkillTarget) => SkillsSummary;
  badgeClass: (status: string) => string;
  makeKey: (name: string, target: SkillTarget) => string;
}>();

const emit = defineEmits<{
  (e: "refresh"): void;
  (e: "openGovernance", prefill?: { name?: string }): void;
  (e: "prevPage"): void;
  (e: "nextPage"): void;
  (e: "toggle", name: string, target: SkillTarget, enable: boolean): void;
  (e: "openVersions", name: string, hasSource: boolean): void;
  (e: "update:filter", value: string): void;
  (e: "update:limit", value: number): void;
}>();

const filterModel = computed({
  get: () => props.filter,
  set: (value: string) => emit("update:filter", value),
});
const limitModel = computed({
  get: () => props.limit,
  set: (value: number) => emit("update:limit", value),
});
const skillsVisible = computed(() => props.data?.skills ?? []);

function statusLabel(status: SkillsSummary["status"]): string {
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

function enableTitle(skill: Skill, target: SkillTarget, canEnable: boolean): string {
  const t = target === "claude_code" ? "Claude Code" : target === "codex" ? "Codex" : "Cursor";
  if (canEnable) return `为 ${t} 启用`;
  const hasSource = !!(skill.source && skill.source.trim());
  if (!hasSource) {
    return `无法启用：缺少来源（source）。请先把技能导入/安装到来源库，再启用到 ${t}。`;
  }
  return `无法启用：目标目录存在未托管的同名条目（冲突/外部/已存在）。请先处理或接管，再启用到 ${t}。`;
}

function disableTitle(target: SkillTarget, canDisable: boolean): string {
  const t = target === "claude_code" ? "Claude Code" : target === "codex" ? "Codex" : "Cursor";
  if (canDisable) return `为 ${t} 禁用`;
  return `无法禁用：目标目录存在未托管的同名条目（请先处理冲突/外部/已存在）`;
}
</script>

<template>
  <div class="skillsBody skillsPageBody skillsPanelBody">
    <div v-if="error" class="modalError">{{ error }}</div>
    <div v-else-if="loading" class="loading">加载中…</div>
    <template v-else>
      <details class="skillsMetaDetails">
        <summary>路径详情</summary>
        <div class="skillsMeta">
          <div class="tinyHint">
            来源：
            <span class="mono">{{ (data?.source_roots ?? []).join(" · ") }}</span>
          </div>
          <div class="tinyHint">
            目标：
            <span class="mono">{{
              (data?.targets ?? []).map((t) => `${t.target}:${t.root}`).join(" · ")
            }}</span>
          </div>
        </div>
      </details>

      <div class="skillsToolbar">
        <input v-model="filterModel" placeholder="搜索技能（名称/路径）…" />
        <label class="skillsLimit">
          <span class="tinyHint">每页</span>
          <select v-model.number="limitModel" :disabled="loading">
            <option :value="50">50</option>
            <option :value="100">100</option>
            <option :value="200">200</option>
            <option :value="500">500</option>
          </select>
        </label>
        <div class="skillsPager">
          <span class="tinyHint mono">{{ rangeLabel }}</span>
          <button
            type="button"
            class="primary"
            @click="emit('openGovernance')"
            :disabled="loading"
            title="添加技能（本地/Git/接管/同步/从源更新）"
          >
            添加技能
          </button>
          <button
            type="button"
            @click="emit('prevPage')"
            :disabled="loading || !canPrev"
            title="上一页"
          >
            上一页
          </button>
          <button
            type="button"
            @click="emit('nextPage')"
            :disabled="loading || !canNext"
            title="下一页"
          >
            下一页
          </button>
          <button
            type="button"
            @click="emit('refresh')"
            :disabled="loading"
            title="刷新"
          >
            刷新
          </button>
        </div>
      </div>

      <div class="skillsTable">
        <div class="skillsHead">
          <div>技能</div>
          <div>Cursor</div>
          <div>Claude Code</div>
          <div>Codex</div>
        </div>

        <div class="skillsRows">
          <div v-for="s in skillsVisible" :key="s.name" class="skillsRow">
            <div class="skillsName">
              <div class="skillsNameTop">
                <div class="mono">{{ s.name }}</div>
                <button
                  type="button"
                  class="skillActionBtn"
                  @click="emit('openVersions', s.name, !!(s.source && s.source.trim()))"
                  title="管理该技能的版本快照"
                >
                  版本
                </button>
              </div>
              <div class="tinyHint mono" v-if="s.source" :title="s.source">
                {{ s.source }}
              </div>
              <div class="tinyHint warn" v-else>
                缺少来源（请先添加/导入/接管）
                <button
                  type="button"
                  class="skillActionBtn"
                  @click="emit('openGovernance', { name: s.name })"
                  title="为该技能添加来源（本地/Git/接管）"
                >
                  添加来源
                </button>
              </div>
            </div>

            <div class="skillsCell">
              <template v-for="t in [summarizeTarget(s, 'cursor')]" :key="t.target">
                <span
                  class="pill mono skillStatus"
                  :class="badgeClass(t.status)"
                  :title="t.detail"
                  >{{ statusLabel(t.status) }}</span
                >
                <button
                  type="button"
                  class="skillActionBtn"
                  v-if="!t.enabled"
                  @click="emit('toggle', s.name, 'cursor', true)"
                  :disabled="!t.canEnable || !!actionBusy.get(makeKey(s.name, 'cursor'))"
                  :title="enableTitle(s, 'cursor', t.canEnable)"
                >
                  {{ actionBusy.get(makeKey(s.name, "cursor")) ? "…" : "启用" }}
                </button>
                <button
                  type="button"
                  class="skillActionBtn"
                  v-else
                  @click="emit('toggle', s.name, 'cursor', false)"
                  :disabled="!t.canDisable || !!actionBusy.get(makeKey(s.name, 'cursor'))"
                  :title="disableTitle('cursor', t.canDisable)"
                >
                  {{ actionBusy.get(makeKey(s.name, "cursor")) ? "…" : "禁用" }}
                </button>
              </template>
            </div>

            <div class="skillsCell">
              <template v-for="t in [summarizeTarget(s, 'claude_code')]" :key="t.target">
                <span
                  class="pill mono skillStatus"
                  :class="badgeClass(t.status)"
                  :title="t.detail"
                  >{{ statusLabel(t.status) }}</span
                >
                <button
                  type="button"
                  class="skillActionBtn"
                  v-if="!t.enabled"
                  @click="emit('toggle', s.name, 'claude_code', true)"
                  :disabled="!t.canEnable || !!actionBusy.get(makeKey(s.name, 'claude_code'))"
                  :title="enableTitle(s, 'claude_code', t.canEnable)"
                >
                  {{ actionBusy.get(makeKey(s.name, "claude_code")) ? "…" : "启用" }}
                </button>
                <button
                  type="button"
                  class="skillActionBtn"
                  v-else
                  @click="emit('toggle', s.name, 'claude_code', false)"
                  :disabled="!t.canDisable || !!actionBusy.get(makeKey(s.name, 'claude_code'))"
                  :title="disableTitle('claude_code', t.canDisable)"
                >
                  {{ actionBusy.get(makeKey(s.name, "claude_code")) ? "…" : "禁用" }}
                </button>
              </template>
            </div>

            <div class="skillsCell">
              <template v-for="t in [summarizeTarget(s, 'codex')]" :key="t.target">
                <span
                  class="pill mono skillStatus"
                  :class="badgeClass(t.status)"
                  :title="t.detail"
                  >{{ statusLabel(t.status) }}</span
                >
                <button
                  type="button"
                  class="skillActionBtn"
                  v-if="!t.enabled"
                  @click="emit('toggle', s.name, 'codex', true)"
                  :disabled="!t.canEnable || !!actionBusy.get(makeKey(s.name, 'codex'))"
                  :title="enableTitle(s, 'codex', t.canEnable)"
                >
                  {{ actionBusy.get(makeKey(s.name, "codex")) ? "…" : "启用" }}
                </button>
                <button
                  type="button"
                  class="skillActionBtn"
                  v-else
                  @click="emit('toggle', s.name, 'codex', false)"
                  :disabled="!t.canDisable || !!actionBusy.get(makeKey(s.name, 'codex'))"
                  :title="disableTitle('codex', t.canDisable)"
                >
                  {{ actionBusy.get(makeKey(s.name, "codex")) ? "…" : "禁用" }}
                </button>
              </template>
            </div>
          </div>

          <div v-if="!skillsVisible.length" class="empty">暂无技能</div>
        </div>
      </div>
    </template>
  </div>
</template>
