<script setup lang="ts">
import { computed, ref, watch } from "vue";
import type {
  AuthStatus,
  ProviderActiveSelection,
  ProviderProfile,
  ProviderSpeedTestResult,
} from "../types";

type ChatBackend = "auto" | "simple-http" | "claude" | "codex";
type ProviderTarget = "claude" | "codex" | "secretary";
type ProvidersPage = "overview" | ProviderTarget;
type SpeedTestTarget = "claude" | "codex" | "";

const props = defineProps<{
  loading: boolean;
  saving: boolean;
  error: string;
  storagePath: string;
  authStatus: AuthStatus | null;
  profiles: ProviderProfile[];
  active: ProviderActiveSelection;
  chatBackend: ChatBackend;

  editID: string;
  editName: string;

  claudeBaseURL: string;
  claudeApiKey: string;
  claudeAuthToken: string;
  claudeModel: string;
  claudeSmallFastModel: string;
  claudeSyncLive: boolean;
  claudeApiKeyHint: string;
  claudeAuthTokenHint: string;

  codexBaseURL: string;
  codexApiKey: string;
  codexModel: string;
  codexReasoningEffort: string;
  codexSyncLive: boolean;
  codexApiKeyHint: string;

  secretarySimpleHTTPBaseURL: string;
  secretarySimpleHTTPApiKey: string;
  secretarySimpleHTTPAuthToken: string;
  secretarySimpleHTTPModel: string;
  secretarySimpleHTTPApiKeyHint: string;
  secretarySimpleHTTPAuthTokenHint: string;

  speedTesting: boolean;
  speedTestTarget: SpeedTestTarget;
  claudeSpeedTest: ProviderSpeedTestResult | null;
  codexSpeedTest: ProviderSpeedTestResult | null;
}>();

const emit = defineEmits<{
  (e: "close"): void;
  (e: "newProfile"): void;
  (e: "refresh"): void;
  (e: "importLive"): void;
  (e: "importEnv", target: "claude" | "codex" | "secretary"): void;
  (e: "export", includeSecrets: boolean): void;
  (e: "speedtest", target: "claude" | "codex"): void;
  (e: "selectProfile", profile: ProviderProfile): void;
  (e: "delete"): void;
  (e: "save", target: "claude" | "codex" | "secretary"): void;
  (e: "activate", target: "claude" | "codex" | "secretary"): void;

  (e: "update:editName", value: string): void;
  (e: "update:claudeBaseURL", value: string): void;
  (e: "update:claudeApiKey", value: string): void;
  (e: "update:claudeAuthToken", value: string): void;
  (e: "update:claudeModel", value: string): void;
  (e: "update:claudeSmallFastModel", value: string): void;
  (e: "update:claudeSyncLive", value: boolean): void;

  (e: "update:codexBaseURL", value: string): void;
  (e: "update:codexApiKey", value: string): void;
  (e: "update:codexModel", value: string): void;
  (e: "update:codexReasoningEffort", value: string): void;
  (e: "update:codexSyncLive", value: boolean): void;

  (e: "update:secretarySimpleHTTPBaseURL", value: string): void;
  (e: "update:secretarySimpleHTTPApiKey", value: string): void;
  (e: "update:secretarySimpleHTTPAuthToken", value: string): void;
  (e: "update:secretarySimpleHTTPModel", value: string): void;

  (e: "update:chatBackend", value: ChatBackend): void;
}>();

const editNameModel = computed({
  get: () => props.editName,
  set: (value: string) => emit("update:editName", value),
});

const claudeBaseURLModel = computed({
  get: () => props.claudeBaseURL,
  set: (value: string) => emit("update:claudeBaseURL", value),
});
const claudeApiKeyModel = computed({
  get: () => props.claudeApiKey,
  set: (value: string) => emit("update:claudeApiKey", value),
});
const claudeAuthTokenModel = computed({
  get: () => props.claudeAuthToken,
  set: (value: string) => emit("update:claudeAuthToken", value),
});
const claudeModelModel = computed({
  get: () => props.claudeModel,
  set: (value: string) => emit("update:claudeModel", value),
});
const claudeSmallFastModelModel = computed({
  get: () => props.claudeSmallFastModel,
  set: (value: string) => emit("update:claudeSmallFastModel", value),
});
const claudeSyncLiveModel = computed({
  get: () => props.claudeSyncLive,
  set: (value: boolean) => emit("update:claudeSyncLive", value),
});

const codexBaseURLModel = computed({
  get: () => props.codexBaseURL,
  set: (value: string) => emit("update:codexBaseURL", value),
});
const codexApiKeyModel = computed({
  get: () => props.codexApiKey,
  set: (value: string) => emit("update:codexApiKey", value),
});
const codexModelModel = computed({
  get: () => props.codexModel,
  set: (value: string) => emit("update:codexModel", value),
});
const codexReasoningEffortModel = computed({
  get: () => props.codexReasoningEffort,
  set: (value: string) => emit("update:codexReasoningEffort", value),
});
const codexSyncLiveModel = computed({
  get: () => props.codexSyncLive,
  set: (value: boolean) => emit("update:codexSyncLive", value),
});

const secretarySimpleHTTPBaseURLModel = computed({
  get: () => props.secretarySimpleHTTPBaseURL,
  set: (value: string) => emit("update:secretarySimpleHTTPBaseURL", value),
});
const secretarySimpleHTTPApiKeyModel = computed({
  get: () => props.secretarySimpleHTTPApiKey,
  set: (value: string) => emit("update:secretarySimpleHTTPApiKey", value),
});
const secretarySimpleHTTPAuthTokenModel = computed({
  get: () => props.secretarySimpleHTTPAuthToken,
  set: (value: string) => emit("update:secretarySimpleHTTPAuthToken", value),
});
const secretarySimpleHTTPModelModel = computed({
  get: () => props.secretarySimpleHTTPModel,
  set: (value: string) => emit("update:secretarySimpleHTTPModel", value),
});

const chatBackendModel = computed({
  get: () => props.chatBackend,
  set: (value: ChatBackend) => emit("update:chatBackend", value),
});

const page = ref<ProvidersPage>("overview");

function targetLabel(t: ProviderTarget): string {
  switch (t) {
    case "claude":
      return "Claude Code";
    case "codex":
      return "Codex";
    case "secretary":
      return "秘书";
    default:
      return String(t);
  }
}

function profileLabel(p: ProviderProfile | null | undefined): string {
  const name = String(p?.name ?? "").trim();
  if (name) return name;
  const id = String(p?.id ?? "").trim();
  return id || "未命名";
}

function profileForID(id: string): ProviderProfile | null {
  const key = String(id ?? "").trim();
  if (!key) return null;
  return props.profiles.find((p) => p.id === key) ?? null;
}

function hasText(value: unknown): boolean {
  return String(value ?? "").trim() !== "";
}

function normalizeTool(value: unknown): ProviderTarget | null {
  const v = String(value ?? "").trim();
  if (v === "claude" || v === "codex" || v === "secretary") return v;
  return null;
}

function hasClaudeTargetData(p: ProviderProfile | null | undefined): boolean {
  if (!p) return false;
  const claude = p.targets?.claude;
  return (
    hasText(claude?.base_url) ||
    hasText(claude?.api_key) ||
    hasText(claude?.auth_token) ||
    hasText(claude?.model) ||
    hasText(claude?.small_fast_model)
  );
}

function hasCodexTargetData(p: ProviderProfile | null | undefined): boolean {
  if (!p) return false;
  const codex = p.targets?.codex;
  return (
    hasText(codex?.base_url) ||
    hasText(codex?.api_key) ||
    hasText(codex?.model) ||
    hasText(codex?.reasoning_effort)
  );
}

function hasSecretaryTargetData(p: ProviderProfile | null | undefined): boolean {
  if (!p) return false;
  const secretary = p.targets?.secretary;
  const backend = String(secretary?.backend ?? "").trim();
  if (backend === "simple-http" || backend === "claude" || backend === "codex") return true;
  return (
    hasText(secretary?.simple_http?.base_url) ||
    hasText(secretary?.simple_http?.api_key) ||
    hasText(secretary?.simple_http?.auth_token) ||
    hasText(secretary?.simple_http?.model)
  );
}

function inferProfileTool(p: ProviderProfile | null | undefined): ProviderTarget | null {
  if (!p) return null;
  if (hasClaudeTargetData(p)) return "claude";
  if (hasCodexTargetData(p)) return "codex";
  if (hasSecretaryTargetData(p)) return "secretary";
  return null;
}

function profileTool(p: ProviderProfile | null | undefined): ProviderTarget | null {
  if (!p) return null;
  return normalizeTool(p.tool) ?? inferProfileTool(p);
}

function activeIDFor(t: ProviderTarget): string {
  if (t === "claude") return String(props.active?.claude ?? "").trim();
  if (t === "codex") return String(props.active?.codex ?? "").trim();
  return String(props.active?.secretary ?? "").trim();
}

function activeProfileFor(t: ProviderTarget): ProviderProfile | null {
  const p = profileForID(activeIDFor(t));
  if (!profileMatchesTarget(p, t)) return null;
  return p;
}

function profileMatchesTarget(p: ProviderProfile | null | undefined, t: ProviderTarget): boolean {
  return profileTool(p) === t;
}

function profilesForTarget(t: ProviderTarget): ProviderProfile[] {
  return props.profiles.filter((p) => profileMatchesTarget(p, t));
}

const pageProfiles = computed<ProviderProfile[]>(() => {
  if (page.value === "overview") return props.profiles;
  return profilesForTarget(page.value);
});

function ensureEditorFor(t: ProviderTarget) {
  const p = activeProfileFor(t);
  if (p) {
    emit("selectProfile", p);
    return;
  }
  const currentEditing = profileForID(props.editID);
  if (currentEditing && profileMatchesTarget(currentEditing, t)) {
    emit("selectProfile", currentEditing);
    return;
  }
  const targetProfiles = profilesForTarget(t);
  const fallback = targetProfiles[0];
  if (fallback) {
    emit("selectProfile", fallback);
    return;
  }
  emit("newProfile");
  editNameModel.value = `${targetLabel(t)} Provider`;
}

function openOverview() {
  page.value = "overview";
}

function openTargetPage(t: ProviderTarget) {
  page.value = t;
  ensureEditorFor(t);
}

function startNewForTarget(t: ProviderTarget) {
  page.value = t;
  emit("newProfile");
  editNameModel.value = `${targetLabel(t)} Provider`;
  if (t === "claude") {
    claudeBaseURLModel.value = "https://api.anthropic.com";
    return;
  }
  if (t === "codex") {
    codexBaseURLModel.value = "https://api.openai.com";
    return;
  }
  secretarySimpleHTTPBaseURLModel.value = "https://api.anthropic.com";
}

function onSelectEditProfile(id: string) {
  if (page.value === "overview") return;
  const p = profileForID(id);
  if (!p || !profileMatchesTarget(p, page.value)) return;
  emit("selectProfile", p);
}

function onSelectActiveProfile(id: string) {
  if (page.value === "overview") return;
  const p = profileForID(id);
  if (!p || !profileMatchesTarget(p, page.value)) return;
  emit("selectProfile", p);
  emit("activate", page.value);
}

function onSaveProfile() {
  if (page.value === "overview") return;
  emit("save", page.value);
}

function onSaveAndActivate() {
  if (page.value === "overview") return;
  emit("activate", page.value);
}

const showSecretaryHTTPNotice = computed<boolean>(() => {
  return props.chatBackend === "auto" || props.chatBackend === "simple-http";
});

watch(
  () => props.loading,
  (loading) => {
    if (loading) return;
    if (page.value === "overview") return;
    ensureEditorFor(page.value);
  },
  { immediate: true },
);

watch(
  () => page.value,
  (next) => {
    if (props.loading) return;
    if (next === "overview") return;
    ensureEditorFor(next);
  },
);
</script>

<template>
  <section class="panel providersPagePanel providersPage">
    <div class="providersHeader">
      <div class="providersHeaderLead">
        <div class="providersTitle">Providers</div>
        <div class="providersSubtitle tinyHint">配置授权 / 模型，并保存启用到 Claude Code、Codex、秘书</div>
      </div>

      <div class="providersHeaderActions">
        <button type="button" class="headerMiniBtn" @click="openOverview()" :disabled="loading || saving || page === 'overview'">
          总览
        </button>
        <button type="button" class="headerMiniBtn" @click="emit('refresh')" :disabled="loading || saving">
          刷新
        </button>
        <button type="button" class="headerMiniBtn" @click="emit('importLive')" :disabled="loading || saving">
          从 AUTH 文件导入
        </button>
        <button type="button" class="headerMiniBtn" @click="emit('export', false)" :disabled="loading || saving">
          导出
        </button>
        <button type="button" class="headerMiniBtn" @click="emit('export', true)" :disabled="loading || saving">
          导出密钥
        </button>
        <button type="button" class="headerMiniBtn" @click="emit('close')">
          返回
        </button>
      </div>
    </div>

    <div class="providersBody">
      <div v-if="storagePath" class="providersMeta">
        <div v-if="storagePath" class="settingsMeta providersStorage">
          存储位置: <span class="mono">{{ storagePath }}</span>
        </div>
      </div>

      <div v-if="error" class="modalError">{{ error }}</div>
      <div v-else-if="loading" class="loading providersLoading">加载中...</div>
      <template v-else>
        <div class="providersSplit">
          <div v-if="page !== 'overview'" class="providersNav" role="tablist" aria-label="工具配置切换">
            <button
              type="button"
              role="tab"
              class="providersNavItem"
              :class="{ active: page === 'claude' }"
              :aria-selected="page === 'claude'"
              @click="openTargetPage('claude')"
            >
              Claude Code
            </button>
            <button
              type="button"
              role="tab"
              class="providersNavItem"
              :class="{ active: page === 'codex' }"
              :aria-selected="page === 'codex'"
              @click="openTargetPage('codex')"
            >
              Codex
            </button>
            <button
              type="button"
              role="tab"
              class="providersNavItem"
              :class="{ active: page === 'secretary' }"
              :aria-selected="page === 'secretary'"
              @click="openTargetPage('secretary')"
            >
              秘书
            </button>
          </div>

          <div class="providersEditor toolsEditor">
            <template v-if="page === 'overview'">
              <div class="providersOverview">
                <div class="providersOverviewTitle">我想配置</div>
                <div class="tinyHint">
                  1) 选择工具 → 2) 填写授权与模型 → 3) 点击“保存并启用”（立即生效，仅影响后续新 run）
                </div>

                <div class="providersOverviewGrid">
                  <button type="button" class="providersOverviewCard" @click="openTargetPage('claude')">
                    <div class="providersOverviewCardTitle">Claude Code</div>
                    <div class="providersOverviewCardMeta tinyHint">
                      <div>
                        当前启用：<span class="mono">{{ profileLabel(activeProfileFor('claude')) }}</span>
                      </div>
                      <div>
                        鉴权状态：<span class="mono">{{ authStatus?.claude?.available ? "可用" : "未配置" }}</span>
                      </div>
                    </div>
                    <div class="providersOverviewCardActions">
                      <span class="primaryPill">配置授权 / 模型</span>
                    </div>
                  </button>

                  <button type="button" class="providersOverviewCard" @click="openTargetPage('codex')">
                    <div class="providersOverviewCardTitle">Codex</div>
                    <div class="providersOverviewCardMeta tinyHint">
                      <div>
                        当前启用：<span class="mono">{{ profileLabel(activeProfileFor('codex')) }}</span>
                      </div>
                      <div>
                        鉴权状态：<span class="mono">{{ authStatus?.codex?.available ? "可用" : "未配置" }}</span>
                      </div>
                    </div>
                    <div class="providersOverviewCardActions">
                      <span class="primaryPill">配置授权 / 模型</span>
                    </div>
                  </button>

                  <button type="button" class="providersOverviewCard" @click="openTargetPage('secretary')">
                    <div class="providersOverviewCardTitle">秘书</div>
                    <div class="providersOverviewCardMeta tinyHint">
                      <div>
                        当前启用：<span class="mono">{{ profileLabel(activeProfileFor('secretary')) }}</span>
                      </div>
                      <div>
                        后端：<span class="mono">{{ chatBackendModel }}</span>
                      </div>
                    </div>
                    <div class="providersOverviewCardActions">
                      <span class="primaryPill">选择后端 / 配置</span>
                    </div>
                  </button>
                </div>

                <div class="providersOverviewHint tinyHint">
                  提示：如你已有本机 AUTH 文件，可直接点右上角“从 AUTH 文件导入”自动生成一套配置。
                </div>
              </div>
            </template>

            <template v-else>
              <div class="providersEditorHead">
                <div>
                  <div class="providersEditorTitle">
                    {{ targetLabel(page) }} · 配置
                  </div>
                  <div class="tinyHint">
                    当前启用：<span class="mono">{{ profileLabel(activeProfileFor(page)) }}</span>
                    · 编辑中：<span class="mono">{{ editID.trim() ? editID : "未保存" }}</span>
                    · 保存并启用：立即生效（仅影响后续新 run）
                  </div>
                </div>

                <div class="providersEditorHeadActions">
                  <button type="button" @click="startNewForTarget(page)" :disabled="saving">新建配置</button>
                  <button type="button" @click="emit('delete')" :disabled="saving || !editID.trim()">删除配置</button>
                </div>
              </div>

              <div class="toolsEditorGrid providersEditorGrid">
                <div class="providersSubsection">
                  <div class="providersSubsectionTitle">已保存配置（点击切换编辑）</div>
                  <div class="providersProfilesList">
                    <button
                      v-for="p in pageProfiles"
                      :key="p.id"
                      type="button"
                      class="providersProfileBtn"
                      :class="{ active: p.id === editID.trim() }"
                      @click="onSelectEditProfile(p.id)"
                      :disabled="saving"
                    >
                      {{ profileLabel(p) }}
                    </button>
                    <div v-if="!pageProfiles.length" class="tinyHint">暂无已保存配置，请先填写并保存。</div>
                  </div>
                </div>

                <label class="full">
                  配置名称
                  <input v-model="editNameModel" placeholder="例如：Anthropic / OpenAI / My Provider" autocomplete="off" />
                </label>

                <div class="providersSubsection">
                  <div class="providersSubsectionHead">
                    <div class="providersSubsectionTitle">授权</div>
                    <button v-if="!editID.trim()" type="button" @click="emit('importEnv', page)" :disabled="saving">
                      从环境变量填充
                    </button>
                  </div>

                  <div v-if="page === 'claude'" class="toolsEditorGrid providersSubsectionGrid">
                    <label class="full">
                      Base URL
                      <input v-model="claudeBaseURLModel" placeholder="https://api.anthropic.com" autocomplete="off" />
                    </label>
                    <label class="full">
                      Auth Token（优先）
                      <input
                        v-model="claudeAuthTokenModel"
                        type="password"
                        :placeholder="claudeAuthTokenHint ? `留空保留（${claudeAuthTokenHint}）` : '留空保留'"
                        autocomplete="off"
                      />
                    </label>
                    <label class="full">
                      API Key
                      <input
                        v-model="claudeApiKeyModel"
                        type="password"
                        :placeholder="claudeApiKeyHint ? `留空保留（${claudeApiKeyHint}）` : '留空保留'"
                        autocomplete="off"
                      />
                    </label>
                    <label class="settingsToggleRow">
                      <input type="checkbox" v-model="claudeSyncLiveModel" />
                      <span>启用时同步到 CLI 配置（可选）</span>
                    </label>

                    <div class="providerTestRow">
                      <button
                        type="button"
                        @click="emit('speedtest', 'claude')"
                        :disabled="saving || !editID.trim() || speedTesting"
                      >
                        <span v-if="speedTesting && speedTestTarget === 'claude'">测试中...</span>
                        <span v-else>速度测试</span>
                      </button>
                      <div
                        v-if="claudeSpeedTest"
                        class="speedTestResult mono"
                        :class="{ ok: claudeSpeedTest.ok, bad: !claudeSpeedTest.ok }"
                      >
                        <span>{{ claudeSpeedTest.ok ? "OK" : "失败" }}</span>
                        <span v-if="claudeSpeedTest.latency_ms != null">{{ claudeSpeedTest.latency_ms }}ms</span>
                        <span v-if="!claudeSpeedTest.ok && (claudeSpeedTest.hint || claudeSpeedTest.error)">{{
                          claudeSpeedTest.hint || claudeSpeedTest.error
                        }}</span>
                      </div>
                    </div>
                  </div>

                  <div v-else-if="page === 'codex'" class="toolsEditorGrid providersSubsectionGrid">
                    <label class="full">
                      Base URL（可选）
                      <input v-model="codexBaseURLModel" placeholder="https://api.openai.com" autocomplete="off" />
                    </label>
                    <label class="full">
                      API Key
                      <input
                        v-model="codexApiKeyModel"
                        type="password"
                        :placeholder="codexApiKeyHint ? `留空保留（${codexApiKeyHint}）` : '留空保留'"
                        autocomplete="off"
                      />
                    </label>
                    <label class="settingsToggleRow">
                      <input type="checkbox" v-model="codexSyncLiveModel" />
                      <span>启用时同步到 CLI 配置（可选）</span>
                    </label>

                    <div class="providerTestRow">
                      <button
                        type="button"
                        @click="emit('speedtest', 'codex')"
                        :disabled="saving || !editID.trim() || speedTesting"
                      >
                        <span v-if="speedTesting && speedTestTarget === 'codex'">测试中...</span>
                        <span v-else>速度测试</span>
                      </button>
                      <div
                        v-if="codexSpeedTest"
                        class="speedTestResult mono"
                        :class="{ ok: codexSpeedTest.ok, bad: !codexSpeedTest.ok }"
                      >
                        <span>{{ codexSpeedTest.ok ? "OK" : "失败" }}</span>
                        <span v-if="codexSpeedTest.latency_ms != null">{{ codexSpeedTest.latency_ms }}ms</span>
                        <span v-if="!codexSpeedTest.ok && (codexSpeedTest.hint || codexSpeedTest.error)">{{
                          codexSpeedTest.hint || codexSpeedTest.error
                        }}</span>
                      </div>
                    </div>
                  </div>

                  <div v-else class="toolsEditorGrid providersSubsectionGrid">
                    <label class="full">
                      秘书后端（对话/评审使用）
                      <select v-model="chatBackendModel">
                        <option value="auto">auto（优先 HTTP → Claude → Codex）</option>
                        <option value="simple-http">simple-http（独立 HTTP）</option>
                        <option value="claude">claude（复用 Claude Code）</option>
                        <option value="codex">codex（复用 Codex）</option>
                      </select>
                    </label>

                    <div v-if="showSecretaryHTTPNotice">
                      <div class="tinyHint">
                        仅当秘书后端为 <span class="mono">simple-http</span> 或 <span class="mono">auto</span>（优先 HTTP）时，才会使用下方 HTTP 凭据。
                      </div>
                      <div class="providersMinorTitle">Simple HTTP（Anthropic 兼容）</div>
                      <label class="full">
                        Base URL
                        <input v-model="secretarySimpleHTTPBaseURLModel" placeholder="https://api.anthropic.com" autocomplete="off" />
                      </label>
                      <label class="full">
                        Auth Token（优先）
                        <input
                          v-model="secretarySimpleHTTPAuthTokenModel"
                          type="password"
                          :placeholder="secretarySimpleHTTPAuthTokenHint ? `留空保留（${secretarySimpleHTTPAuthTokenHint}）` : '留空保留'"
                          autocomplete="off"
                        />
                      </label>
                      <label class="full">
                        API Key
                        <input
                          v-model="secretarySimpleHTTPApiKeyModel"
                          type="password"
                          :placeholder="secretarySimpleHTTPApiKeyHint ? `留空保留（${secretarySimpleHTTPApiKeyHint}）` : '留空保留'"
                          autocomplete="off"
                        />
                      </label>
                    </div>

                    <div v-else class="tinyHint">
                      当前选择为 <span class="mono">{{ chatBackendModel }}</span>；秘书会复用对应工具的启用配置（AUTH + 模型）。
                    </div>
                  </div>
                </div>

                <div class="providersSubsection">
                  <div class="providersSubsectionTitle">模型</div>

                  <div v-if="page === 'claude'" class="toolsEditorGrid providersSubsectionGrid">
                    <label class="full">
                      主力模型（model）
                      <input v-model="claudeModelModel" placeholder="claude-3-7-sonnet" autocomplete="off" />
                    </label>
                    <label class="full">
                      快速模型（small fast）
                      <input v-model="claudeSmallFastModelModel" placeholder="claude-3-5-haiku" autocomplete="off" />
                    </label>
                  </div>

                  <div v-else-if="page === 'codex'" class="toolsEditorGrid providersSubsectionGrid">
                    <label class="full">
                      模型（model）
                      <input v-model="codexModelModel" placeholder="gpt-5.2" autocomplete="off" />
                    </label>
                    <label class="full">
                      推理强度（reasoning effort）
                      <select v-model="codexReasoningEffortModel">
                        <option value="">默认</option>
                        <option value="low">低</option>
                        <option value="medium">中</option>
                        <option value="high">高</option>
                        <option value="xhigh">很高</option>
                      </select>
                    </label>
                  </div>

                  <div v-else class="toolsEditorGrid providersSubsectionGrid">
                    <div v-if="showSecretaryHTTPNotice">
                      <div class="providersMinorTitle">Simple HTTP（Anthropic 兼容）</div>
                      <label class="full">
                        模型（model）
                        <input v-model="secretarySimpleHTTPModelModel" placeholder="claude-3-5-sonnet-latest" autocomplete="off" />
                      </label>
                    </div>
                    <div v-else class="tinyHint">
                      当前选择为 <span class="mono">{{ chatBackendModel }}</span>；无需在此配置模型。
                    </div>
                  </div>
                </div>

                <div class="providersSubsection">
                  <div class="providersSubsectionTitle">
                    <template v-if="page === 'secretary' && showSecretaryHTTPNotice">启用（用于 simple-http）</template>
                    <template v-else>启用</template>
                  </div>
                  <div v-if="page === 'secretary' && !showSecretaryHTTPNotice" class="tinyHint">
                    当前后端为 <span class="mono">{{ chatBackendModel }}</span>；秘书会复用对应工具的启用配置，无需启用秘书配置。
                    如需使用 <span class="mono">simple-http</span>，请先切换秘书后端。
                  </div>
                  <div v-else class="toolsEditorGrid providersSubsectionGrid">
                    <label class="full">
                      切换当前启用配置（{{ targetLabel(page) }}，会立即生效）
                      <select
                        :value="activeIDFor(page)"
                        :disabled="saving || !pageProfiles.length"
                        @change="onSelectActiveProfile(($event.target as HTMLSelectElement).value)"
                      >
                        <option value="" disabled>未启用（请选择…）</option>
                        <option v-for="p in pageProfiles" :key="p.id" :value="p.id">
                          {{ profileLabel(p) }}
                        </option>
                      </select>
                    </label>
                  </div>
                </div>

                <div class="providersFooterActions">
                  <button type="button" @click="onSaveProfile" :disabled="saving || !editName.trim()">
                    仅保存
                  </button>
                  <button
                    type="button"
                    class="primary"
                    @click="onSaveAndActivate"
                    :disabled="saving || !editName.trim()"
                    :title="`保存并启用到 ${targetLabel(page)}（立即生效）`"
                  >
                    {{ saving ? "保存中..." : "保存并启用" }}
                  </button>
                </div>
                <div class="tinyHint">提示：保存并启用会立即生效；仅保存不会影响当前使用配置。</div>
              </div>
            </template>
          </div>
        </div>
      </template>
    </div>
  </section>
</template>

<style scoped>
.providersPage {
  min-height: 0;
}

.providersHeader {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  padding: 12px 12px 4px;
}

.providersHeaderLead {
  display: grid;
  gap: 4px;
}

.providersTitle {
  font-weight: 900;
  letter-spacing: 0.01em;
}

.providersHeaderActions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  justify-content: flex-end;
}

.providersBody {
  display: flex;
  flex-direction: column;
  gap: 12px;
  min-height: 0;
  overflow: auto;
  padding: 0 12px 12px;
}

.providersSplit {
  display: flex;
  flex-direction: column;
  gap: 10px;
  flex: 1;
  min-height: 0;
}

.providersNav {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  overflow: auto;
}

.providersNavItem {
  width: auto;
  text-align: center;
  padding: 8px 14px;
  border-radius: 12px;
  border: 1px solid var(--border-color);
  background: transparent;
}

.providersNavItem:hover {
  background: var(--bg-subtle);
}

.providersNavItem.active {
  border-color: rgba(20, 184, 166, 0.35);
  background: rgba(20, 184, 166, 0.08);
}

.providersNavItem:focus-visible {
  outline: 2px solid rgba(20, 184, 166, 0.55);
  outline-offset: 2px;
}

.providersEditor {
  overflow: auto;
}

.providersEditorHead {
  display: flex;
  gap: 12px;
  align-items: flex-start;
  justify-content: space-between;
  margin-bottom: 12px;
}

.providersEditorTitle {
  font-weight: 900;
}

.providersEditorHeadActions {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  justify-content: flex-end;
}

.providersMinorTitle {
  font-weight: 900;
  margin: 6px 0;
}

.providersProfilesList {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.providersProfileBtn {
  border-radius: 999px;
  padding: 8px 12px;
}

.providersProfileBtn.active {
  border-color: rgba(20, 184, 166, 0.35);
  background: rgba(20, 184, 166, 0.08);
}

.providersOverview {
  display: grid;
  gap: 12px;
}

.providersOverviewTitle {
  font-weight: 900;
}

.providersOverviewGrid {
  display: grid;
  gap: 12px;
  grid-template-columns: repeat(3, minmax(0, 1fr));
}

.providersOverviewCard {
  border: 1px solid var(--border-color);
  border-radius: 14px;
  padding: 12px;
  background: var(--bg-subtle);
  text-align: left;
  cursor: pointer;
  min-height: 96px;
}

.providersOverviewCard:hover {
  border-color: rgba(20, 184, 166, 0.35);
  background: rgba(20, 184, 166, 0.06);
}

.providersOverviewCard:focus-visible {
  outline: 2px solid rgba(20, 184, 166, 0.55);
  outline-offset: 2px;
}

.providersOverviewCardTitle {
  font-weight: 900;
  margin-bottom: 6px;
}

.providersOverviewCardMeta {
  display: grid;
  gap: 4px;
}

.providersOverviewCardActions {
  margin-top: 10px;
  display: flex;
  justify-content: flex-end;
}

.primaryPill {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 6px 10px;
  border-radius: 999px;
  background: rgba(20, 184, 166, 0.14);
  border: 1px solid rgba(20, 184, 166, 0.35);
  color: var(--text);
  font-weight: 800;
}

.providersOverviewHint {
  margin-top: 2px;
}

@media (max-width: 820px) {
  .providersOverviewGrid {
    grid-template-columns: 1fr;
  }
}

.providersSection {
  display: grid;
  gap: 12px;
}

.providersSubsection {
  border: 1px solid var(--border-color);
  border-radius: 14px;
  padding: 10px 12px;
  background: var(--bg-subtle);
}

.providersSubsectionTitle {
  font-weight: 900;
  margin-bottom: 6px;
}

.providersSubsectionHead {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  flex-wrap: wrap;
}

.providersSubsectionHead .providersSubsectionTitle {
  margin-bottom: 0;
}

.providersSubsectionGrid {
  margin-top: 10px;
}

.providersFooterActions {
  display: flex;
  gap: 10px;
  justify-content: flex-end;
  align-items: center;
  padding-top: 8px;
}

.providersLoading {
  padding: 20px 0;
}

.providersMeta {
  display: grid;
  gap: 10px;
}

.providersStorage {
  margin: 0;
}

</style>
