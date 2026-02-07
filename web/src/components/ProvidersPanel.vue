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
type ProvidersSection = "tokens" | "models";
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
  (e: "export", includeSecrets: boolean): void;
  (e: "speedtest", target: "claude" | "codex"): void;
  (e: "selectProfile", profile: ProviderProfile): void;
  (e: "delete"): void;
  (e: "save"): void;
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

const section = ref<ProvidersSection>("tokens");
const target = ref<ProviderTarget>("claude");

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

function sectionLabel(s: ProvidersSection): string {
  return s === "tokens" ? "令牌管理" : "模型管理";
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

function activeIDFor(t: ProviderTarget): string {
  if (t === "claude") return String(props.active?.claude ?? "").trim();
  if (t === "codex") return String(props.active?.codex ?? "").trim();
  return String(props.active?.secretary ?? "").trim();
}

function activeProfileFor(t: ProviderTarget): ProviderProfile | null {
  return profileForID(activeIDFor(t));
}

function ensureEditorFor(t: ProviderTarget) {
  const p = activeProfileFor(t);
  if (p) {
    emit("selectProfile", p);
    return;
  }
  const fallback = props.profiles[0];
  if (fallback) {
    emit("selectProfile", fallback);
    return;
  }
  emit("newProfile");
  editNameModel.value = `${targetLabel(t)} Provider`;
}

function selectNav(nextSection: ProvidersSection, nextTarget: ProviderTarget) {
  section.value = nextSection;
  if (target.value === nextTarget) return;
  target.value = nextTarget;
  ensureEditorFor(nextTarget);
}

function activateProfileForTarget(profile: ProviderProfile, t: ProviderTarget) {
  emit("selectProfile", profile);
  emit("activate", t);
}

function startNewForTarget(t: ProviderTarget) {
  target.value = t;
  emit("newProfile");
  editNameModel.value = `${targetLabel(t)} Provider`;
}

function startTemplate(t: ProviderTarget, kind: string) {
  startNewForTarget(t);

  // Safe defaults; users can edit before saving.
  if (t === "claude") {
    if (kind === "anthropic") {
      editNameModel.value = "Anthropic";
      claudeBaseURLModel.value = "https://api.anthropic.com";
      claudeModelModel.value = "claude-3-7-sonnet";
      claudeSmallFastModelModel.value = "claude-3-5-haiku";
    } else if (kind === "anthropic-compatible") {
      editNameModel.value = "Anthropic Compatible";
      claudeBaseURLModel.value = "";
    }
    return;
  }

  if (t === "codex") {
    if (kind === "openai") {
      editNameModel.value = "OpenAI";
      codexBaseURLModel.value = "https://api.openai.com";
      codexModelModel.value = "gpt-5.2";
      codexReasoningEffortModel.value = "";
    } else if (kind === "openai-compatible") {
      editNameModel.value = "OpenAI Compatible";
      codexBaseURLModel.value = "";
    }
    return;
  }

  if (t === "secretary") {
    if (kind === "simple-http") {
      editNameModel.value = "Secretary HTTP";
      chatBackendModel.value = "simple-http";
      secretarySimpleHTTPBaseURLModel.value = "https://api.anthropic.com";
      secretarySimpleHTTPModelModel.value = "claude-3-5-sonnet-latest";
    } else if (kind === "reuse-claude") {
      editNameModel.value = "Secretary (Claude)";
      chatBackendModel.value = "claude";
    } else if (kind === "reuse-codex") {
      editNameModel.value = "Secretary (Codex)";
      chatBackendModel.value = "codex";
    }
  }
}

function onSelectActiveProfile(id: string) {
  const p = profileForID(id);
  if (!p) return;
  activateProfileForTarget(p, target.value);
}

function onSaveAndActivate() {
  emit("activate", target.value);
}

const showSecretaryHTTPNotice = computed<boolean>(() => {
  return props.chatBackend === "auto" || props.chatBackend === "simple-http";
});

watch(
  () => props.loading,
  (loading) => {
    if (loading) return;
    ensureEditorFor(target.value);
  },
  { immediate: true },
);
</script>

<template>
  <section class="panel providersPagePanel providersPage">
    <div class="providersHeader">
      <div class="providersHeaderLead">
        <div class="providersTitle">Providers</div>
        <div class="providersSubtitle tinyHint">令牌管理 / 模型管理 · 保存并启用（立即生效）</div>
      </div>

      <div class="providersHeaderActions">
        <button type="button" class="headerMiniBtn" @click="emit('refresh')" :disabled="loading || saving">
          刷新
        </button>
        <button type="button" class="headerMiniBtn" @click="emit('importLive')" :disabled="loading || saving">
          从 CLI 导入
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
      <div v-if="storagePath || authStatus?.warnings?.length" class="providersMeta">
        <div v-if="storagePath" class="settingsMeta providersStorage">
          存储位置: <span class="mono">{{ storagePath }}</span>
        </div>
        <div v-if="authStatus?.warnings?.length" class="providersWarning">
          <div class="providersWarningTitle">检测到环境变量覆盖</div>
          <div class="tinyHint">
            保存仍会写入配置文件，但运行会优先使用环境变量。若要让“保存并启用”生效，请先清除这些环境变量并重启 ControlCCX。
          </div>
          <div class="providersWarningList mono">
            <div v-for="w in authStatus.warnings" :key="w">{{ w }}</div>
          </div>
        </div>
      </div>

      <div v-if="error" class="modalError">{{ error }}</div>
      <div v-else-if="loading" class="loading providersLoading">加载中...</div>
      <template v-else>
        <div class="providersSplit toolsSplit">
          <div class="providersNav toolsList">
            <div class="providersNavGroup">
              <div class="providersNavTitle">令牌管理</div>
              <button
                type="button"
                class="providersNavItem toolsItem"
                :class="{ active: section === 'tokens' && target === 'claude' }"
                @click="selectNav('tokens', 'claude')"
              >
                Claude Code
              </button>
              <button
                type="button"
                class="providersNavItem toolsItem"
                :class="{ active: section === 'tokens' && target === 'codex' }"
                @click="selectNav('tokens', 'codex')"
              >
                Codex
              </button>
              <button
                type="button"
                class="providersNavItem toolsItem"
                :class="{ active: section === 'tokens' && target === 'secretary' }"
                @click="selectNav('tokens', 'secretary')"
              >
                秘书
              </button>
            </div>

            <div class="providersNavGroup">
              <div class="providersNavTitle">模型管理</div>
              <button
                type="button"
                class="providersNavItem toolsItem"
                :class="{ active: section === 'models' && target === 'claude' }"
                @click="selectNav('models', 'claude')"
              >
                Claude Code
              </button>
              <button
                type="button"
                class="providersNavItem toolsItem"
                :class="{ active: section === 'models' && target === 'codex' }"
                @click="selectNav('models', 'codex')"
              >
                Codex
              </button>
              <button
                type="button"
                class="providersNavItem toolsItem"
                :class="{ active: section === 'models' && target === 'secretary' }"
                @click="selectNav('models', 'secretary')"
              >
                秘书
              </button>
            </div>
          </div>

          <div class="providersEditor toolsEditor">
            <div class="providersEditorHead">
              <div>
                <div class="providersEditorTitle">
                  {{ targetLabel(target) }} · {{ sectionLabel(section) }}
                </div>
                <div class="tinyHint">
                  编辑中：<span class="mono">{{ editID.trim() ? editID : "未保存" }}</span> · 保存并启用：立即生效（仅影响后续新 run）
                </div>
              </div>

              <div class="providersEditorHeadActions">
                <button type="button" @click="startNewForTarget(target)" :disabled="saving">新建</button>
                <button type="button" @click="emit('delete')" :disabled="saving || !editID.trim()">删除</button>
              </div>
            </div>

            <div class="providersQuickTemplates">
              <div class="providersQuickTitle">快速模板</div>
              <div class="providersQuickGrid">
                <template v-if="target === 'claude'">
                  <button type="button" class="providersQuickBtn" @click="startTemplate('claude', 'anthropic')">
                    Anthropic（官方）
                  </button>
                  <button type="button" class="providersQuickBtn" @click="startTemplate('claude', 'anthropic-compatible')">
                    Anthropic 兼容（自定义 Base URL）
                  </button>
                </template>
                <template v-else-if="target === 'codex'">
                  <button type="button" class="providersQuickBtn" @click="startTemplate('codex', 'openai')">
                    OpenAI（官方）
                  </button>
                  <button type="button" class="providersQuickBtn" @click="startTemplate('codex', 'openai-compatible')">
                    OpenAI 兼容（自定义 Base URL）
                  </button>
                </template>
                <template v-else>
                  <button type="button" class="providersQuickBtn" @click="startTemplate('secretary', 'simple-http')">
                    HTTP（独立 Base URL + Key/Token）
                  </button>
                  <button type="button" class="providersQuickBtn" @click="startTemplate('secretary', 'reuse-claude')">
                    复用 Claude Code 授权
                  </button>
                  <button type="button" class="providersQuickBtn" @click="startTemplate('secretary', 'reuse-codex')">
                    复用 Codex 授权
                  </button>
                </template>
              </div>
            </div>

            <div class="toolsEditorGrid providersEditorGrid">
              <label class="full">
                配置名称
                <input
                  v-model="editNameModel"
                  placeholder="例如：Anthropic / OpenAI / My Provider"
                  autocomplete="off"
                />
              </label>

              <label class="full">
                切换当前启用配置（{{ targetLabel(target) }}，会立即生效）
                <select
                  :value="activeIDFor(target)"
                  :disabled="saving || !profiles.length"
                  @change="onSelectActiveProfile(($event.target as HTMLSelectElement).value)"
                >
                  <option value="" disabled>未启用（请选择…）</option>
                  <option v-for="p in profiles" :key="p.id" :value="p.id">
                    {{ profileLabel(p) }}
                  </option>
                </select>
              </label>

              <div v-if="section === 'tokens' && target === 'claude'" class="providersSection">
                <label class="full">
                  Base URL
                  <input v-model="claudeBaseURLModel" placeholder="https://api.anthropic.com" autocomplete="off" />
                </label>
                <label class="full">
                  Auth Token（优先）
                  <input
                    v-model="claudeAuthTokenModel"
                    type="password"
                    :placeholder="
                      claudeAuthTokenHint
                        ? `留空保留（${claudeAuthTokenHint}）`
                        : '留空保留'
                    "
                    autocomplete="off"
                  />
                </label>
                <label class="full">
                  API Key
                  <input
                    v-model="claudeApiKeyModel"
                    type="password"
                    :placeholder="
                      claudeApiKeyHint
                        ? `留空保留（${claudeApiKeyHint}）`
                        : '留空保留'
                    "
                    autocomplete="off"
                  />
                </label>
                <label class="settingsToggleRow">
                  <input type="checkbox" v-model="claudeSyncLiveModel" />
                  <span>启用时同步到 CLI 配置（可选）</span>
                </label>
              </div>

              <div v-else-if="section === 'tokens' && target === 'codex'" class="providersSection">
                <label class="full">
                  Base URL（可选）
                  <input v-model="codexBaseURLModel" placeholder="https://api.openai.com" autocomplete="off" />
                </label>
                <label class="full">
                  API Key
                  <input
                    v-model="codexApiKeyModel"
                    type="password"
                    :placeholder="
                      codexApiKeyHint
                        ? `留空保留（${codexApiKeyHint}）`
                        : '留空保留'
                    "
                    autocomplete="off"
                  />
                </label>
                <label class="settingsToggleRow">
                  <input type="checkbox" v-model="codexSyncLiveModel" />
                  <span>启用时同步到 CLI 配置（可选）</span>
                </label>
              </div>

              <div v-else-if="section === 'tokens' && target === 'secretary'" class="providersSection">
                <div class="tinyHint">
                  仅当“秘书后端”选择为 <span class="mono">simple-http</span>（或 <span class="mono">auto</span> 优先 HTTP）时，才会使用这里的 HTTP 凭据。
                </div>

                <div class="providersSubsection">
                  <div class="providersSubsectionTitle">Simple HTTP（Anthropic 兼容）</div>
                  <div class="toolsEditorGrid providersSubsectionGrid">
                    <label class="full">
                      Base URL
                      <input
                        v-model="secretarySimpleHTTPBaseURLModel"
                        placeholder="https://api.anthropic.com"
                        autocomplete="off"
                      />
                    </label>
                    <label class="full">
                      Auth Token（优先）
                      <input
                        v-model="secretarySimpleHTTPAuthTokenModel"
                        type="password"
                        :placeholder="
                          secretarySimpleHTTPAuthTokenHint
                            ? `留空保留（${secretarySimpleHTTPAuthTokenHint}）`
                            : '留空保留'
                        "
                        autocomplete="off"
                      />
                    </label>
                    <label class="full">
                      API Key
                      <input
                        v-model="secretarySimpleHTTPApiKeyModel"
                        type="password"
                        :placeholder="
                          secretarySimpleHTTPApiKeyHint
                            ? `留空保留（${secretarySimpleHTTPApiKeyHint}）`
                            : '留空保留'
                        "
                        autocomplete="off"
                      />
                    </label>
                  </div>
                </div>
              </div>

              <div v-else-if="section === 'models' && target === 'claude'" class="providersSection">
                <label class="full">
                  模型（model）
                  <input v-model="claudeModelModel" placeholder="claude-3-7-sonnet" autocomplete="off" />
                </label>
                <label class="full">
                  小模型（快速）
                  <input v-model="claudeSmallFastModelModel" placeholder="claude-3-5-haiku" autocomplete="off" />
                </label>
              </div>

              <div v-else-if="section === 'models' && target === 'codex'" class="providersSection">
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

              <div v-else class="providersSection">
                <label class="full">
                  秘书后端（对话/评审使用）
                  <select v-model="chatBackendModel">
                    <option value="auto">auto（优先 HTTP → Claude → Codex）</option>
                    <option value="simple-http">simple-http（独立 HTTP）</option>
                    <option value="claude">claude（复用 Claude Code）</option>
                    <option value="codex">codex（复用 Codex）</option>
                  </select>
                </label>

                <div v-if="showSecretaryHTTPNotice" class="providersSubsection">
                  <div class="providersSubsectionTitle">Simple HTTP（Anthropic 兼容）</div>
                  <div class="tinyHint">
                    Base URL / Key / Token 在“令牌管理 → 秘书”里填写；这里仅设置模型。
                  </div>
                  <label class="full">
                    模型（model）
                    <input
                      v-model="secretarySimpleHTTPModelModel"
                      placeholder="claude-3-5-sonnet-latest"
                      autocomplete="off"
                    />
                  </label>
                </div>

                <div v-else class="tinyHint">
                  当前选择为 <span class="mono">{{ chatBackendModel }}</span>；秘书会复用对应工具的启用配置。
                </div>
              </div>

              <div v-if="section === 'tokens' && (target === 'claude' || target === 'codex')" class="providerTestRow">
                <button
                  type="button"
                  @click="emit('speedtest', target === 'claude' ? 'claude' : 'codex')"
                  :disabled="saving || !editID.trim() || speedTesting"
                >
                  <span v-if="speedTesting && speedTestTarget === target">测试中...</span>
                  <span v-else>速度测试</span>
                </button>
                <div
                  v-if="target === 'claude' ? claudeSpeedTest : codexSpeedTest"
                  class="speedTestResult mono"
                  :class="{ ok: (target === 'claude' ? claudeSpeedTest : codexSpeedTest)?.ok, bad: !(target === 'claude' ? claudeSpeedTest : codexSpeedTest)?.ok }"
                >
                  <span>{{ (target === 'claude' ? claudeSpeedTest : codexSpeedTest)?.ok ? "OK" : "失败" }}</span>
                  <span v-if="(target === 'claude' ? claudeSpeedTest : codexSpeedTest)?.latency_ms != null">
                    {{ (target === 'claude' ? claudeSpeedTest : codexSpeedTest)?.latency_ms }}ms
                  </span>
                  <span
                    v-if="!(target === 'claude' ? claudeSpeedTest : codexSpeedTest)?.ok && ((target === 'claude' ? claudeSpeedTest : codexSpeedTest)?.hint || (target === 'claude' ? claudeSpeedTest : codexSpeedTest)?.error)"
                  >
                    {{ (target === 'claude' ? claudeSpeedTest : codexSpeedTest)?.hint || (target === 'claude' ? claudeSpeedTest : codexSpeedTest)?.error }}
                  </span>
                </div>
              </div>

              <div class="providersFooterActions">
                <button type="button" @click="emit('save')" :disabled="saving || !editName.trim()">
                  仅保存
                </button>
                <button
                  type="button"
                  class="primary"
                  @click="onSaveAndActivate"
                  :disabled="saving || !editName.trim()"
                  title="保存并启用到当前目标（立即生效）"
                >
                  {{ saving ? "保存中..." : "保存并启用" }}
                </button>
              </div>
              <div class="tinyHint">提示：保存并启用会立即生效；仅保存不会影响当前使用配置。</div>
            </div>
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
  flex: 1;
  min-height: 0;
}

.providersNav {
  overflow: auto;
}

.providersNavTitle {
  padding: 2px 2px 6px;
  font-size: 12px;
  font-weight: 800;
  color: var(--text-sub);
}

.providersNavGroup + .providersNavGroup {
  margin-top: 10px;
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

.providersQuickTemplates {
  border: 1px solid var(--border-color);
  border-radius: 14px;
  padding: 10px 12px;
  background: var(--bg-subtle);
  margin-bottom: 12px;
}

.providersQuickTitle {
  font-weight: 900;
  margin-bottom: 8px;
}

.providersQuickGrid {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
}

.providersQuickBtn {
  border-radius: 999px;
  padding: 8px 12px;
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

.providersWarning {
  border: 1px solid color-mix(in srgb, var(--border-color) 80%, rgba(245, 158, 11, 0.25) 20%);
  background: color-mix(in srgb, var(--bg-subtle) 78%, rgba(245, 158, 11, 0.06) 22%);
  border-radius: 14px;
  padding: 10px 12px;
  display: grid;
  gap: 6px;
}

.providersWarningTitle {
  font-weight: 900;
}
</style>
