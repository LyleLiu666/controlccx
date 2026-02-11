<script setup lang="ts">
import { computed } from "vue";
import type { Skill, SkillRepoFacet, SkillsListResponse } from "../types";
import { buildSkillsRepoView } from "../skillsRepoGrouping";

type SkillTarget = "cursor" | "claude_code" | "codex" | "antigravity" | "opencode";
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
  repoFilter: string;
  groupByRepo: boolean;
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
  (e: "takeover", name: string, target: SkillTarget): void;
  (e: "openVersions", name: string, hasSource: boolean): void;
  (e: "update:filter", value: string): void;
  (e: "update:repoFilter", value: string): void;
  (e: "update:groupByRepo", value: boolean): void;
  (e: "update:limit", value: number): void;
}>();

const filterModel = computed({
  get: () => props.filter,
  set: (value: string) => emit("update:filter", value),
});
const repoFilterModel = computed({
  get: () => props.repoFilter,
  set: (value: string) => emit("update:repoFilter", value),
});
const groupByRepoModel = computed({
  get: () => props.groupByRepo,
  set: (value: boolean) => emit("update:groupByRepo", value),
});
const limitModel = computed({
  get: () => props.limit,
  set: (value: number) => emit("update:limit", value),
});
const repoOptions = computed<SkillRepoFacet[]>(() => props.data?.repos ?? []);
const repoView = computed(() =>
  buildSkillsRepoView({
    skills: props.data?.skills ?? [],
    q: props.filter,
    repo: props.repoFilter,
    groupByRepo: props.groupByRepo,
  }),
);
const targetsOrdered = computed(() => {
  const seen = new Set<SkillTarget>();
  for (const t of props.data?.targets ?? []) {
    seen.add(t.target as SkillTarget);
  }

  const preferred: SkillTarget[] = ["claude_code", "codex", "antigravity", "opencode", "cursor"];
  const out: SkillTarget[] = [];
  for (const t of preferred) {
    if (!seen.has(t)) continue;
    out.push(t);
    seen.delete(t);
  }
  const rest = Array.from(seen).sort((a, b) => String(a).localeCompare(String(b)));
  return out.concat(rest);
});

function targetLabel(target: SkillTarget): string {
  switch (target) {
    case "claude_code":
      return "Claude Code";
    case "codex":
      return "Codex";
    case "antigravity":
      return "Antigravity";
    case "opencode":
      return "OpenCode";
    case "cursor":
      return "Cursor";
    default:
      return String(target);
  }
}

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
  const t = targetLabel(target);
  const hasSource = !!(skill.source && skill.source.trim());
  const hasBootstrapCandidate = (skill.targets ?? []).some(
    (s) => s.status === "present" || s.status === "external" || s.status === "copied" || s.status === "linked",
  );
  if (canEnable) {
    if (!hasSource) return `为 ${t} 启用（将自动接管来源）`;
    return `为 ${t} 启用`;
  }
  if (!hasSource) {
    if (!hasBootstrapCandidate) {
      return `无法启用：缺少来源（source），且未发现可接管的现有技能目录。请先添加来源，再启用到 ${t}。`;
    }
    return `无法启用：该技能缺少来源但可自动接管；当前目标目录存在未托管的同名条目（冲突/外部/已存在）。请先处理或接管，再启用到 ${t}。`;
  }
  return `无法启用：目标目录存在未托管的同名条目（冲突/外部/已存在）。请先处理或接管，再启用到 ${t}。`;
}

function disableTitle(target: SkillTarget, canDisable: boolean): string {
  const t = targetLabel(target);
  if (canDisable) return `为 ${t} 禁用`;
  return `无法禁用：目标目录存在未托管的同名条目（请先处理冲突/外部/已存在）`;
}

function canTakeover(skill: Skill, t: SkillsSummary): boolean {
  const hasSource = !!(skill.source && skill.source.trim());
  if (!hasSource) return false;
  return t.status === "present" || t.status === "external" || t.status === "conflict";
}

function takeoverTitle(target: SkillTarget): string {
  const t = targetLabel(target);
  return `接管 ${t} 目标中的同名条目（将覆盖并改为受控关联）`;
}

function newVersionTitle(skill: Skill): string {
  const at = String(skill.new_version_at ?? "").trim();
  if (at) return `已自动生成新快照（不自动切换）。${at}`;
  return "已自动生成新快照（不自动切换）。点击“版本”查看。";
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
        <label class="skillsRepoFilter">
          <span class="tinyHint">Repo</span>
          <select v-model="repoFilterModel" :disabled="loading">
            <option value="">All repos</option>
            <option v-for="repo in repoOptions" :key="repo.key" :value="repo.key" :title="repo.ref ?? ''">
              {{ repo.label }} ({{ repo.count }})
            </option>
          </select>
        </label>
        <label class="skillsGroupToggle" title="按仓库分组展示（仅显示 Git 来源技能）">
          <input type="checkbox" v-model="groupByRepoModel" />
          <span class="tinyHint">按 repo 分组</span>
        </label>
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
          <div>目标</div>
        </div>

        <div class="skillsRows">
          <template v-if="groupByRepoModel">
            <template v-for="group in repoView.groups" :key="group.key">
              <div class="skillsRepoGroupHead" :title="group.ref || group.label">
                <span class="mono">{{ group.label }}</span>
                <span class="tinyHint">({{ group.skills.length }})</span>
              </div>

              <div v-for="s in group.skills" :key="`${group.key}:${s.name}`" class="skillsRow">
                <div class="skillsName">
                  <div class="skillsNameTop">
                    <div class="mono">{{ s.name }}</div>
                    <span
                      v-if="s.new_version"
                      class="pill warn mono newVersionPill"
                      :title="newVersionTitle(s)"
                      >新版本</span
                    >
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
                    缺少来源（可直接在目标里点“启用”自动接管；或先添加/导入/接管）
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
                  <template v-for="target in targetsOrdered" :key="target">
                    <template v-for="t in [summarizeTarget(s, target)]" :key="t.target">
                      <div class="skillsTargetBlock">
                        <span class="tinyHint mono skillsTargetLabel">{{ targetLabel(target) }}</span>
                        <span
                          class="pill mono skillStatus"
                          :class="badgeClass(t.status)"
                          :title="t.detail"
                          >{{ statusLabel(t.status) }}</span
                        >
                        <button
                          type="button"
                          class="skillActionBtn"
                          v-if="!t.enabled && t.canEnable"
                          @click="emit('toggle', s.name, target, true)"
                          :disabled="!!actionBusy.get(makeKey(s.name, target))"
                          :title="enableTitle(s, target, true)"
                        >
                          {{ actionBusy.get(makeKey(s.name, target)) ? "…" : "启用" }}
                        </button>
                        <button
                          v-else-if="!t.enabled && canTakeover(s, t)"
                          type="button"
                          class="skillActionBtn"
                          @click="emit('takeover', s.name, target)"
                          :disabled="!!actionBusy.get(makeKey(s.name, target))"
                          :title="takeoverTitle(target)"
                        >
                          {{ actionBusy.get(makeKey(s.name, target)) ? "…" : "接管" }}
                        </button>
                        <button
                          v-else-if="!t.enabled"
                          type="button"
                          class="skillActionBtn"
                          :disabled="true"
                          :title="enableTitle(s, target, t.canEnable)"
                        >
                          启用
                        </button>
                        <button
                          type="button"
                          class="skillActionBtn"
                          v-else
                          @click="emit('toggle', s.name, target, false)"
                          :disabled="!t.canDisable || !!actionBusy.get(makeKey(s.name, target))"
                          :title="disableTitle(target, t.canDisable)"
                        >
                          {{ actionBusy.get(makeKey(s.name, target)) ? "…" : "禁用" }}
                        </button>
                      </div>
                    </template>
                  </template>
                </div>
              </div>
            </template>
          </template>

          <template v-else>
            <div v-for="s in repoView.items" :key="s.name" class="skillsRow">
              <div class="skillsName">
                <div class="skillsNameTop">
                  <div class="mono">{{ s.name }}</div>
                  <span
                    v-if="s.new_version"
                    class="pill warn mono newVersionPill"
                    :title="newVersionTitle(s)"
                    >新版本</span
                  >
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
                  缺少来源（可直接在目标里点“启用”自动接管；或先添加/导入/接管）
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
                <template v-for="target in targetsOrdered" :key="target">
                  <template v-for="t in [summarizeTarget(s, target)]" :key="t.target">
                    <div class="skillsTargetBlock">
                      <span class="tinyHint mono skillsTargetLabel">{{ targetLabel(target) }}</span>
                      <span
                        class="pill mono skillStatus"
                        :class="badgeClass(t.status)"
                        :title="t.detail"
                        >{{ statusLabel(t.status) }}</span
                      >
                      <button
                        type="button"
                        class="skillActionBtn"
                        v-if="!t.enabled && t.canEnable"
                        @click="emit('toggle', s.name, target, true)"
                        :disabled="!!actionBusy.get(makeKey(s.name, target))"
                        :title="enableTitle(s, target, true)"
                      >
                        {{ actionBusy.get(makeKey(s.name, target)) ? "…" : "启用" }}
                      </button>
                      <button
                        v-else-if="!t.enabled && canTakeover(s, t)"
                        type="button"
                        class="skillActionBtn"
                        @click="emit('takeover', s.name, target)"
                        :disabled="!!actionBusy.get(makeKey(s.name, target))"
                        :title="takeoverTitle(target)"
                      >
                        {{ actionBusy.get(makeKey(s.name, target)) ? "…" : "接管" }}
                      </button>
                      <button
                        v-else-if="!t.enabled"
                        type="button"
                        class="skillActionBtn"
                        :disabled="true"
                        :title="enableTitle(s, target, t.canEnable)"
                      >
                        启用
                      </button>
                      <button
                        type="button"
                        class="skillActionBtn"
                        v-else
                        @click="emit('toggle', s.name, target, false)"
                        :disabled="!t.canDisable || !!actionBusy.get(makeKey(s.name, target))"
                        :title="disableTitle(target, t.canDisable)"
                      >
                        {{ actionBusy.get(makeKey(s.name, target)) ? "…" : "禁用" }}
                      </button>
                    </div>
                  </template>
                </template>
              </div>
            </div>
          </template>

          <div v-if="!repoView.items.length" class="empty">
            {{ groupByRepoModel ? "分组模式下暂无 Git 来源技能，可关闭“按 repo 分组”查看全部。" : "暂无技能" }}
          </div>
        </div>
      </div>
    </template>
  </div>
</template>
