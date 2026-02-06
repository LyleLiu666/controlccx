<script setup lang="ts">
import { computed, ref } from "vue";
import type {
  AuthStatus,
  ProviderActiveSelection,
  ProviderProfile,
  ProviderSpeedTestResult,
} from "../types";

type SecretaryBackend = "auto" | "simple-http" | "claude" | "codex";
type ChatBackend = "auto" | "simple-http" | "claude" | "codex";
type SpeedTestTarget = "claude" | "codex" | "";
type ProviderTarget = "claude" | "codex" | "secretary";

const props = defineProps<{
  open: boolean;
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

  secretaryBackend: SecretaryBackend;
  secretarySimpleHTTPBaseURL: string;
  secretarySimpleHTTPApiKey: string;
  secretarySimpleHTTPAuthToken: string;
  secretarySimpleHTTPModel: string;
  secretarySimpleHTTPApiKeyHint: string;
  secretarySimpleHTTPAuthTokenHint: string;
  secretarySyncLive: boolean;

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

  (e: "update:editID", value: string): void;
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

  (e: "update:secretaryBackend", value: SecretaryBackend): void;
  (e: "update:secretarySimpleHTTPBaseURL", value: string): void;
  (e: "update:secretarySimpleHTTPApiKey", value: string): void;
  (e: "update:secretarySimpleHTTPAuthToken", value: string): void;
  (e: "update:secretarySimpleHTTPModel", value: string): void;
  (e: "update:secretarySyncLive", value: boolean): void;

  (e: "update:chatBackend", value: ChatBackend): void;
}>();

const editIDModel = computed({
  get: () => props.editID,
  set: (value: string) => emit("update:editID", value),
});
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

const secretaryBackendModel = computed({
  get: () => props.secretaryBackend,
  set: (value: SecretaryBackend) => emit("update:secretaryBackend", value),
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
const secretarySyncLiveModel = computed({
  get: () => props.secretarySyncLive,
  set: (value: boolean) => emit("update:secretarySyncLive", value),
});

const chatBackendModel = computed({
  get: () => props.chatBackend,
  set: (value: ChatBackend) => emit("update:chatBackend", value),
});

const editorTab = ref<ProviderTarget>("claude");
const mainTab = ref<"bind" | "library">("bind");

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

function activeProfileFor(target: ProviderTarget): ProviderProfile | null {
  if (target === "claude") return profileForID(String(props.active?.claude ?? ""));
  if (target === "codex") return profileForID(String(props.active?.codex ?? ""));
  return profileForID(String(props.active?.secretary ?? ""));
}

function activateProfileForTarget(profile: ProviderProfile, target: ProviderTarget) {
  editorTab.value = target;
  emit("selectProfile", profile);
  emit("activate", target);
}

function bindProfileID(target: ProviderTarget, id: string) {
  const p = profileForID(id);
  if (!p) return;
  activateProfileForTarget(p, target);
}

function startNewForTarget(target: ProviderTarget) {
  mainTab.value = "library";
  editorTab.value = target;
  emit("newProfile");
}

function startNewTemplate(target: ProviderTarget, kind: string) {
  startNewForTarget(target);

  // Best-effort, safe defaults. Users can edit before saving.
  if (target === "claude") {
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

  if (target === "codex") {
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

  // Secretary uses the global chat backend choice; only simple-http needs stored creds.
  if (target === "secretary") {
    if (kind === "simple-http") {
      editNameModel.value = "Secretary Simple HTTP";
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

function editActiveForTarget(target: ProviderTarget) {
  const p = activeProfileFor(target);
  mainTab.value = "library";
  editorTab.value = target;
  if (p) emit("selectProfile", p);
}
</script>

<template>
  <div v-if="open" class="modalOverlay" @click.self="emit('close')">
    <div class="modal toolsModal providersModal">
      <div class="modalHeader providersHeader">
          <div class="providersHeaderLead">
            <div class="modalTitle">提供方</div>
          <div class="providersHeaderHint">管理 Claude Code / Codex / 秘书 的 Provider profiles。</div>
          </div>
        <div class="providersHeaderActions">
          <button type="button" class="headerMiniBtn" @click="startNewForTarget('claude')">
            新建
          </button>
          <button
            type="button"
            class="headerMiniBtn"
            @click="emit('refresh')"
            :disabled="loading || saving"
          >
            刷新
          </button>
          <button
            type="button"
            class="headerMiniBtn"
            @click="emit('importLive')"
            :disabled="loading || saving"
          >
            从 CLI 导入
          </button>
          <button
            type="button"
            class="headerMiniBtn"
            @click="emit('export', false)"
            :disabled="loading || saving"
          >
            导出
          </button>
          <button
            type="button"
            class="headerMiniBtn"
            @click="emit('export', true)"
            :disabled="loading || saving"
          >
            导出密钥
          </button>
        </div>
        <button class="iconBtn providersCloseBtn" type="button" @click="emit('close')">✕</button>
      </div>

      <div class="modalBody toolsBody providersBody">
        <div
          v-if="storagePath || authStatus?.warnings?.length"
          class="providersMeta"
        >
          <div v-if="storagePath" class="settingsMeta providersStorage">
            存储位置: <span class="mono">{{ storagePath }}</span>
          </div>
          <div v-if="authStatus?.warnings?.length" class="providersWarning">
            <div class="providersWarningTitle">检测到环境变量覆盖</div>
            <div class="tinyHint">
              如果你希望“挂钩/启用”生效，请先取消这些环境变量并重启 ControlCCX。
            </div>
            <div class="providersWarningList mono">
              <div v-for="w in authStatus.warnings" :key="w">{{ w }}</div>
            </div>
          </div>
        </div>

        <div v-if="error" class="modalError">{{ error }}</div>
        <div v-else-if="loading" class="loading providersLoading">加载中...</div>
        <template v-else>
          <div class="providersMainTabs" role="tablist" aria-label="Providers view">
            <button
              type="button"
              class="providersMainTab"
              :class="{ on: mainTab === 'bind' }"
              @click="mainTab = 'bind'"
            >
              挂钩
            </button>
            <button
              type="button"
              class="providersMainTab"
              :class="{ on: mainTab === 'library' }"
              @click="mainTab = 'library'"
            >
              提供方库
            </button>
          </div>

          <div v-if="mainTab === 'bind'" class="providersBind">
            <div class="providersCatalog">
              <div class="providersCatalogTitle">支持的提供方类型</div>
              <div class="tinyHint">
                你可以先创建/导入提供方，再把它挂钩到 Claude Code、Codex 和秘书（只影响后续新 run）。
              </div>
              <div class="providersCatalogGrid">
                <div class="providersCatalogGroup">
                  <div class="providersCatalogGroupTitle">Claude Code</div>
                  <div class="providersCatalogItems">
                    <button type="button" class="providersCatalogItem" @click="startNewTemplate('claude', 'anthropic')">
                      Anthropic（官方）
                    </button>
                    <button
                      type="button"
                      class="providersCatalogItem"
                      @click="startNewTemplate('claude', 'anthropic-compatible')"
                    >
                      Anthropic 兼容（自定义 Base URL）
                    </button>
                    <button type="button" class="providersCatalogItem" @click="emit('importLive')">
                      从 CLI 导入（Claude/Codex）
                    </button>
                  </div>
                </div>

                <div class="providersCatalogGroup">
                  <div class="providersCatalogGroupTitle">Codex</div>
                  <div class="providersCatalogItems">
                    <button type="button" class="providersCatalogItem" @click="startNewTemplate('codex', 'openai')">
                      OpenAI（官方）
                    </button>
                    <button
                      type="button"
                      class="providersCatalogItem"
                      @click="startNewTemplate('codex', 'openai-compatible')"
                    >
                      OpenAI 兼容（自定义 Base URL）
                    </button>
                    <button type="button" class="providersCatalogItem" @click="emit('importLive')">
                      从 CLI 导入（Claude/Codex）
                    </button>
                  </div>
                </div>

                <div class="providersCatalogGroup">
                  <div class="providersCatalogGroupTitle">秘书</div>
                  <div class="providersCatalogItems">
                    <button type="button" class="providersCatalogItem" @click="startNewTemplate('secretary', 'simple-http')">
                      HTTP（独立 Base URL + Key/Token）
                    </button>
                    <button type="button" class="providersCatalogItem" @click="startNewTemplate('secretary', 'reuse-claude')">
                      复用 Claude Code 授权
                    </button>
                    <button type="button" class="providersCatalogItem" @click="startNewTemplate('secretary', 'reuse-codex')">
                      复用 Codex 授权
                    </button>
                  </div>
                </div>
              </div>
            </div>

            <div class="providersBindGrid">
              <div class="providersBindCard">
                <div class="providersBindTitleRow">
                  <div class="providersBindTitle">Claude Code</div>
                  <div class="tinyHint">
                    当前挂钩：<span class="mono">{{ profileLabel(activeProfileFor('claude')) }}</span>
                  </div>
                </div>
                <label class="full">
                  选择已保存的提供方
                  <select
                    :value="String(active?.claude ?? '').trim() || ''"
                    @change="bindProfileID('claude', ($event.target as HTMLSelectElement).value)"
                  >
                    <option value="" disabled>请选择…</option>
                    <option v-for="p in profiles" :key="p.id" :value="p.id">
                      {{ profileLabel(p) }}
                    </option>
                  </select>
                </label>
                <div class="providersBindActions">
                  <button type="button" class="primary" @click="startNewForTarget('claude')">新增并配置</button>
                  <button
                    v-if="activeProfileFor('claude')"
                    type="button"
                    @click="editActiveForTarget('claude')"
                  >
                    编辑当前
                  </button>
                </div>
              </div>

              <div class="providersBindCard">
                <div class="providersBindTitleRow">
                  <div class="providersBindTitle">Codex</div>
                  <div class="tinyHint">
                    当前挂钩：<span class="mono">{{ profileLabel(activeProfileFor('codex')) }}</span>
                  </div>
                </div>
                <label class="full">
                  选择已保存的提供方
                  <select
                    :value="String(active?.codex ?? '').trim() || ''"
                    @change="bindProfileID('codex', ($event.target as HTMLSelectElement).value)"
                  >
                    <option value="" disabled>请选择…</option>
                    <option v-for="p in profiles" :key="p.id" :value="p.id">
                      {{ profileLabel(p) }}
                    </option>
                  </select>
                </label>
                <div class="providersBindActions">
                  <button type="button" class="primary" @click="startNewForTarget('codex')">新增并配置</button>
                  <button
                    v-if="activeProfileFor('codex')"
                    type="button"
                    @click="editActiveForTarget('codex')"
                  >
                    编辑当前
                  </button>
                </div>
              </div>

              <div class="providersBindCard">
                <div class="providersBindTitleRow">
                  <div class="providersBindTitle">秘书</div>
                  <div class="tinyHint">
                    当前后端：<span class="mono">{{ chatBackendModel }}</span>
                  </div>
                </div>

                <label class="full">
                  秘书后端（对话/评审使用）
                  <select v-model="chatBackendModel">
                    <option value="auto">auto（优先 HTTP → Claude → Codex）</option>
                    <option value="simple-http">simple-http（独立 HTTP）</option>
                    <option value="claude">claude（复用 Claude Code）</option>
                    <option value="codex">codex（复用 Codex）</option>
                  </select>
                </label>

                <label v-if="chatBackendModel === 'auto' || chatBackendModel === 'simple-http'" class="full">
                  选择 HTTP 提供方（用于 simple-http）
                  <select
                    :value="String(active?.secretary ?? '').trim() || ''"
                    @change="bindProfileID('secretary', ($event.target as HTMLSelectElement).value)"
                  >
                    <option value="" disabled>请选择…</option>
                    <option v-for="p in profiles" :key="p.id" :value="p.id">
                      {{ profileLabel(p) }}
                    </option>
                  </select>
                </label>

                <div v-else class="tinyHint">
                  选择 <span class="mono">claude</span>/<span class="mono">codex</span> 时，秘书会复用对应工具的挂钩配置。
                </div>

                <div class="providersBindActions">
                  <button type="button" class="primary" @click="startNewForTarget('secretary')">新增并配置</button>
                  <button
                    v-if="activeProfileFor('secretary')"
                    type="button"
                    @click="editActiveForTarget('secretary')"
                  >
                    编辑 HTTP 提供方
                  </button>
                </div>
              </div>
            </div>

            <div class="tinyHint">
              提示：挂钩只影响后续新 run；已在运行的任务不会被打断或重启。
            </div>
          </div>

          <div v-else class="toolsSplit providersSplit">
            <div class="toolsList providersList">
              <div class="providersListTitleRow">
                <div class="tinyHint">提供方 profiles</div>
                <div class="tinyHint providersListLegend">
                  点击右侧按钮即可挂钩到：Claude Code / Codex / 秘书
                </div>
              </div>

              <div v-if="!profiles.length" class="tinyHint providersEmpty">
                还没有提供方 profiles。你可以先在“挂钩”里选择类型创建，或点右上角“新建”。
              </div>

              <div
                v-for="p in profiles"
                :key="p.id"
                class="providersItem"
                :class="{ active: p.id === editID }"
              >
                <button
                  type="button"
                  class="providersItemSelect"
                  @click="emit('selectProfile', p)"
                >
                    <div class="providersItemHead">
                      <div class="mono providersItemName">{{ p.name || p.id }}</div>
                    <span v-if="p.id === editID" class="providersItemTag">编辑中</span>
                    </div>
                  <div class="tinyHint mono providersItemID">{{ p.id }}</div>
                </button>

                <div class="providersTargetsSummary">
                  <div class="tinyHint">已挂钩到</div>
                  <div class="providersTargetsBadges">
                    <span v-if="active.claude === p.id" class="providersTargetBadge">Claude Code</span>
                    <span v-if="active.codex === p.id" class="providersTargetBadge">Codex</span>
                    <span v-if="active.secretary === p.id" class="providersTargetBadge">秘书</span>
                    <span
                      v-if="active.claude !== p.id && active.codex !== p.id && active.secretary !== p.id"
                      class="tinyHint"
                      >未挂钩</span
                    >
                  </div>
                  <button type="button" class="providersTargetsAction" @click="mainTab = 'bind'">
                    去挂钩
                  </button>
                </div>
              </div>
            </div>

            <div class="toolsEditor providersEditor">
              <div class="toolsEditorGrid">
                <label class="full">
                  名称
                  <input
                    v-model="editNameModel"
                    placeholder="例如：Anthropic / OpenAI / My Provider"
                    autocomplete="off"
                  />
                </label>

                <div class="providersTabs" role="tablist" aria-label="Provider target">
                  <button
                    type="button"
                    class="providersTab"
                    :class="{ on: editorTab === 'claude' }"
                    @click="editorTab = 'claude'"
                  >
                    Claude Code
                  </button>
                  <button
                    type="button"
                    class="providersTab"
                    :class="{ on: editorTab === 'codex' }"
                    @click="editorTab = 'codex'"
                  >
                    Codex
                  </button>
                  <button
                    type="button"
                    class="providersTab"
                    :class="{ on: editorTab === 'secretary' }"
                    @click="editorTab = 'secretary'"
                  >
                    秘书
                  </button>
                </div>

                <div v-if="editorTab === 'claude'" class="providersTabPanel">
                  <div class="toolsEditorGrid">
                    <label class="full">
                      Base URL
                      <input
                        v-model="claudeBaseURLModel"
                        placeholder="https://api.anthropic.com"
                        autocomplete="off"
                      />
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
                    <label class="full">
                      模型（model）
                      <input
                        v-model="claudeModelModel"
                        placeholder="claude-3-7-sonnet"
                        autocomplete="off"
                      />
                    </label>
                    <label class="full">
                      小模型（快速）
                      <input
                        v-model="claudeSmallFastModelModel"
                        placeholder="claude-3-5-haiku"
                        autocomplete="off"
                      />
                    </label>
                    <label class="settingsToggleRow">
                      <input type="checkbox" v-model="claudeSyncLiveModel" />
                      <span>挂钩时同步到 CLI 配置（可选）</span>
                    </label>
                    <div class="providerTestRow">
                      <button
                        type="button"
                        @click="emit('speedtest', 'claude')"
                        :disabled="
                          saving ||
                          !editID.trim() ||
                          speedTesting
                        "
                      >
                        <span
                          v-if="speedTesting && speedTestTarget === 'claude'"
                          >测试中...</span
                        >
                        <span v-else>速度测试</span>
                      </button>
                      <div
                        v-if="claudeSpeedTest"
                        class="speedTestResult mono"
                        :class="{ ok: claudeSpeedTest.ok, bad: !claudeSpeedTest.ok }"
                      >
                        <span>{{ claudeSpeedTest.ok ? "OK" : "失败" }}</span>
                        <span v-if="claudeSpeedTest.latency_ms != null">
                          {{ claudeSpeedTest.latency_ms }}ms</span
                        >
                        <span
                          v-if="
                            !claudeSpeedTest.ok &&
                            (claudeSpeedTest.hint || claudeSpeedTest.error)
                          "
                        >
                          {{ claudeSpeedTest.hint || claudeSpeedTest.error }}</span
                        >
                      </div>
                    </div>
                  </div>
                </div>

                <div v-else-if="editorTab === 'codex'" class="providersTabPanel">
                  <div class="toolsEditorGrid">
                    <label class="full">
                      Base URL（可选）
                      <input
                        v-model="codexBaseURLModel"
                        placeholder="https://api.openai.com"
                        autocomplete="off"
                      />
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
                    <label class="full">
                      模型（model）
                      <input
                        v-model="codexModelModel"
                        placeholder="gpt-5.2"
                        autocomplete="off"
                      />
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
                    <label class="settingsToggleRow">
                      <input type="checkbox" v-model="codexSyncLiveModel" />
                      <span>挂钩时同步到 CLI 配置（可选）</span>
                    </label>
                    <div class="providerTestRow">
                      <button
                        type="button"
                        @click="emit('speedtest', 'codex')"
                        :disabled="
                          saving ||
                          !editID.trim() ||
                          speedTesting
                        "
                      >
                        <span
                          v-if="speedTesting && speedTestTarget === 'codex'"
                          >测试中...</span
                        >
                        <span v-else>速度测试</span>
                      </button>
                      <div
                        v-if="codexSpeedTest"
                        class="speedTestResult mono"
                        :class="{ ok: codexSpeedTest.ok, bad: !codexSpeedTest.ok }"
                      >
                        <span>{{ codexSpeedTest.ok ? "OK" : "失败" }}</span>
                        <span v-if="codexSpeedTest.latency_ms != null">
                          {{ codexSpeedTest.latency_ms }}ms</span
                        >
                        <span
                          v-if="
                            !codexSpeedTest.ok &&
                            (codexSpeedTest.hint || codexSpeedTest.error)
                          "
                        >
                          {{ codexSpeedTest.hint || codexSpeedTest.error }}</span
                        >
                      </div>
                    </div>
                  </div>
                </div>

                <div v-else class="providersTabPanel">
                  <div class="toolsEditorGrid">
                    <div class="tinyHint">
                      仅当“秘书后端”选择为 <span class="mono">simple-http</span>（或 <span class="mono">auto</span> 优先 HTTP）时，
                      会使用这里的 HTTP 凭据。
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
                        <label class="full">
                          模型（model）
                          <input
                            v-model="secretarySimpleHTTPModelModel"
                            placeholder="claude-3-5-sonnet-latest"
                            autocomplete="off"
                          />
                        </label>
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </template>
      </div>

      <div class="modalFooter">
        <button type="button" @click="emit('close')">关闭</button>
        <button type="button" @click="emit('delete')" :disabled="saving || !editID.trim()">
          删除
        </button>
        <button
          type="button"
          class="primary"
          @click="emit('save')"
          title="仅保存（不切换当前工具）"
          :disabled="saving || !editName.trim()"
        >
          {{ saving ? "保存中..." : "仅保存" }}
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.providersModal {
  width: min(1160px, 96vw);
  height: min(760px, 92vh);
  border-radius: 20px;
  background:
    radial-gradient(1100px 460px at 8% -14%, rgba(20, 184, 166, 0.09), transparent 55%),
    radial-gradient(760px 380px at 100% -30%, rgba(56, 189, 248, 0.1), transparent 56%),
    var(--bg-panel);
}

.providersHeader {
  align-items: flex-start;
  gap: 12px;
  border-bottom-color: color-mix(in srgb, var(--border-color) 78%, transparent);
  background:
    linear-gradient(180deg, color-mix(in srgb, var(--bg-subtle) 92%, white 8%), var(--bg-subtle));
}

.providersHeaderLead {
  display: grid;
  gap: 4px;
  min-width: 220px;
}

.providersHeaderHint {
  font-size: 12px;
  color: var(--text-sub);
  letter-spacing: 0.01em;
}

.providersHeaderActions {
  margin-left: auto;
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  justify-content: flex-end;
}

.providersHeaderActions .headerMiniBtn {
  min-height: 36px;
  padding: 8px 14px;
  border-radius: 999px;
  border-color: color-mix(in srgb, var(--border-color) 80%, rgba(20, 184, 166, 0.35) 20%);
  background: color-mix(in srgb, var(--bg-panel) 88%, transparent);
}

.providersHeaderActions .headerMiniBtn:hover:not(:disabled) {
  border-color: color-mix(in srgb, var(--color-primary) 36%, var(--border-color));
  background: color-mix(in srgb, var(--color-primary-bg) 40%, var(--bg-panel));
}

.providersCloseBtn {
  margin-left: 2px;
}

.providersBody {
  display: flex;
  flex-direction: column;
  gap: 12px;
  min-height: 0;
  overflow: auto;
}

.providersMeta {
  display: grid;
  gap: 10px;
}

.providersStorage {
  margin: 0;
  border: 1px dashed color-mix(in srgb, var(--border-color) 76%, rgba(20, 184, 166, 0.28) 24%);
  border-radius: 12px;
  background: color-mix(in srgb, var(--bg-subtle) 92%, white 8%);
  padding: 10px 12px;
  color: var(--text-sub);
}

.providersLoading {
  display: grid;
  place-items: center;
  border: 1px dashed var(--border-color);
  border-radius: 14px;
  min-height: 160px;
  color: var(--text-sub);
}

.providersMainTabs {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  padding: 6px;
  border: 1px solid color-mix(in srgb, var(--border-color) 76%, rgba(20, 184, 166, 0.2) 24%);
  border-radius: 14px;
  background: color-mix(in srgb, var(--bg-panel) 82%, transparent);
}

.providersMainTab {
  flex: 1 1 160px;
  min-height: 36px;
  border-radius: 999px;
  border: 1px solid transparent;
  background: transparent;
  color: var(--text-sub);
  font-weight: 900;
}

.providersMainTab:hover:not(.on) {
  background: color-mix(in srgb, var(--bg-subtle) 70%, transparent);
  color: var(--text-main);
}

.providersMainTab.on {
  border-color: color-mix(in srgb, var(--color-primary) 46%, transparent);
  background: color-mix(in srgb, var(--color-primary-bg) 56%, transparent);
  color: var(--text-main);
}

.providersBind {
  display: grid;
  gap: 12px;
}

.providersCatalog {
  border: 1px solid color-mix(in srgb, var(--border-color) 76%, rgba(20, 184, 166, 0.2) 24%);
  border-radius: 16px;
  padding: 12px;
  background:
    linear-gradient(
      180deg,
      color-mix(in srgb, var(--bg-panel) 90%, rgba(20, 184, 166, 0.06)),
      color-mix(in srgb, var(--bg-subtle) 92%, rgba(14, 116, 144, 0.08))
    );
}

.providersCatalogTitle {
  font-weight: 900;
  color: var(--text-main);
}

.providersCatalogGrid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 10px;
  margin-top: 10px;
}

.providersCatalogGroup {
  border: 1px solid color-mix(in srgb, var(--border-color) 84%, rgba(20, 184, 166, 0.16) 16%);
  border-radius: 14px;
  padding: 10px;
  background: color-mix(in srgb, var(--bg-panel) 86%, transparent);
  display: grid;
  gap: 10px;
  min-width: 0;
}

.providersCatalogGroupTitle {
  font-weight: 900;
  color: var(--text-main);
}

.providersCatalogItems {
  display: grid;
  gap: 8px;
}

.providersCatalogItem {
  text-align: left;
  padding: 10px 12px;
  border-radius: 14px;
  border: 1px solid color-mix(in srgb, var(--border-color) 80%, rgba(20, 184, 166, 0.18) 20%);
  background: color-mix(in srgb, var(--bg-subtle) 86%, transparent);
  color: var(--text-main);
  font-weight: 800;
}

.providersCatalogItem:hover {
  border-color: color-mix(in srgb, var(--color-primary) 36%, var(--border-color));
  background: color-mix(in srgb, var(--color-primary-bg) 30%, var(--bg-panel));
}

.providersBindGrid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 10px;
}

.providersBindCard {
  border: 1px solid color-mix(in srgb, var(--border-color) 84%, rgba(20, 184, 166, 0.16) 16%);
  border-radius: 16px;
  padding: 12px;
  background: color-mix(in srgb, var(--bg-subtle) 93%, white 7%);
  display: grid;
  gap: 10px;
  min-width: 0;
}

.providersBindTitleRow {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
  align-items: baseline;
}

.providersBindTitle {
  font-weight: 900;
  color: var(--text-main);
}

.providersBindActions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  flex-wrap: wrap;
}

.providersSplit {
  grid-template-columns: minmax(240px, 300px) minmax(0, 1fr);
  flex: 1;
  min-height: 0;
}

.providersList {
  border-radius: 16px;
  border-color: color-mix(in srgb, var(--border-color) 85%, rgba(20, 184, 166, 0.2) 15%);
  background: color-mix(in srgb, var(--bg-subtle) 93%, white 7%);
  padding: 10px;
  min-height: 0;
  overflow: auto;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.providersListTitleRow {
  display: grid;
  gap: 4px;
  padding: 2px 2px 6px;
}

.providersListLegend {
  color: var(--text-sub);
}

.providersEmpty {
  padding: 8px 2px;
}

.providersItem {
  border: 1px solid transparent;
  border-radius: 12px;
  padding: 10px 12px;
  background: color-mix(in srgb, var(--bg-panel) 84%, transparent);
  display: grid;
  gap: 10px;
  transition: background-color 0.18s ease, border-color 0.18s ease, box-shadow 0.18s ease;
}

.providersItem:hover {
  border-color: color-mix(in srgb, var(--border-color) 60%, rgba(20, 184, 166, 0.38) 40%);
  background: color-mix(in srgb, var(--bg-subtle) 58%, var(--bg-panel));
}

.providersItem.active {
  border-color: color-mix(in srgb, var(--color-primary) 48%, transparent);
  background:
    linear-gradient(
      135deg,
      color-mix(in srgb, var(--color-primary-bg) 56%, transparent),
      color-mix(in srgb, var(--bg-panel) 80%, transparent)
    );
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--color-primary) 20%, transparent);
}

.providersItemSelect {
  width: 100%;
  border: none;
  background: transparent;
  padding: 0;
  text-align: left;
  cursor: pointer;
  display: grid;
  gap: 6px;
}

.providersItemSelect:disabled {
  cursor: not-allowed;
  opacity: 0.7;
}

.providersItemHead {
  display: flex;
  align-items: center;
  gap: 8px;
}

.providersItemName {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
}

.providersItemID {
  color: var(--text-sub);
}

.providersItemTag {
  margin-left: auto;
  border-radius: 999px;
  border: 1px solid color-mix(in srgb, var(--color-primary) 32%, transparent);
  background: color-mix(in srgb, var(--color-primary-bg) 55%, transparent);
  color: var(--text-main);
  font-size: 10px;
  font-weight: 800;
  line-height: 1;
  padding: 4px 7px;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.providersWarning {
  border: 1px solid color-mix(in srgb, rgba(245, 158, 11, 0.6) 62%, var(--border-color));
  background: color-mix(in srgb, rgba(245, 158, 11, 0.14) 82%, var(--bg-panel));
  border-radius: 14px;
  padding: 12px 14px;
  margin: 0;
}

.providersWarningTitle {
  font-weight: 900;
  color: var(--text-main);
  margin-bottom: 6px;
}

.providersWarningList {
  margin-top: 8px;
  opacity: 0.95;
}

.providersTargetsSummary {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.providersTargetsBadges {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
  align-items: center;
  min-width: 0;
}

.providersTargetBadge {
  min-height: 24px;
  padding: 4px 8px;
  border-radius: 999px;
  border: 1px solid color-mix(in srgb, var(--border-color) 80%, rgba(20, 184, 166, 0.2) 20%);
  background: color-mix(in srgb, var(--color-primary-bg) 44%, var(--bg-panel));
  color: var(--text-main);
  font-weight: 900;
  font-size: 11px;
  letter-spacing: 0.02em;
}

.providersTargetsAction {
  margin-left: auto;
  min-height: 30px;
  padding: 6px 10px;
  border-radius: 999px;
  border-color: color-mix(in srgb, var(--border-color) 78%, rgba(20, 184, 166, 0.26) 24%);
  background: color-mix(in srgb, var(--bg-panel) 88%, transparent);
  color: var(--text-sub);
  font-weight: 900;
  font-size: 12px;
}

.providersTargetsAction:hover {
  border-color: color-mix(in srgb, var(--color-primary) 36%, var(--border-color));
  background: color-mix(in srgb, var(--color-primary-bg) 32%, var(--bg-panel));
  color: var(--text-main);
}

.providersEditor {
  border-radius: 16px;
  border-color: color-mix(in srgb, var(--border-color) 84%, rgba(20, 184, 166, 0.2) 16%);
  background: color-mix(in srgb, var(--bg-subtle) 93%, white 7%);
  padding: 14px;
  min-height: 0;
  overflow: auto;
}

.providersTabs {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  padding: 6px;
  border: 1px solid color-mix(in srgb, var(--border-color) 76%, rgba(20, 184, 166, 0.2) 24%);
  border-radius: 14px;
  background: color-mix(in srgb, var(--bg-panel) 78%, transparent);
}

.providersTab {
  flex: 1 1 120px;
  min-height: 34px;
  border-radius: 999px;
  border: 1px solid transparent;
  background: transparent;
  color: var(--text-sub);
  font-weight: 900;
}

.providersTab:hover:not(.on) {
  background: color-mix(in srgb, var(--bg-subtle) 70%, transparent);
  color: var(--text-main);
}

.providersTab.on {
  border-color: color-mix(in srgb, var(--color-primary) 46%, transparent);
  background: color-mix(in srgb, var(--color-primary-bg) 56%, transparent);
  color: var(--text-main);
}

.providersTabPanel {
  border: 1px solid color-mix(in srgb, var(--border-color) 76%, rgba(20, 184, 166, 0.2) 24%);
  border-radius: 14px;
  padding: 12px;
  background: color-mix(in srgb, var(--bg-panel) 84%, transparent);
}

.providersSubsection {
  border: 1px dashed color-mix(in srgb, var(--border-color) 76%, rgba(20, 184, 166, 0.28) 24%);
  border-radius: 14px;
  padding: 10px 12px;
  background: color-mix(in srgb, var(--bg-panel) 88%, transparent);
  display: grid;
  gap: 8px;
}

.providersSubsectionTitle {
  font-weight: 900;
  color: var(--text-main);
}

.providersSubsectionGrid {
  margin-top: 4px;
}

.providerTestRow {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.speedTestResult {
  padding: 4px 8px;
  border-radius: 999px;
  border: 1px solid var(--border-color);
  background: rgba(255, 255, 255, 0.04);
  color: var(--text-muted);
  font-size: 12px;
  line-height: 1.2;
}

.speedTestResult.ok {
  border-color: rgba(20, 184, 166, 0.45);
  background: rgba(20, 184, 166, 0.12);
  color: var(--text-main);
}

.speedTestResult.bad {
  border-color: rgba(248, 113, 113, 0.4);
  background: rgba(248, 113, 113, 0.08);
  color: var(--text-main);
}

.providersList,
.providersEditor {
  scrollbar-width: thin;
  scrollbar-color: color-mix(in srgb, var(--text-sub) 55%, transparent) transparent;
}

.providersList::-webkit-scrollbar,
.providersEditor::-webkit-scrollbar {
  width: 10px;
  height: 10px;
}

.providersList::-webkit-scrollbar-thumb,
.providersEditor::-webkit-scrollbar-thumb {
  background: color-mix(in srgb, var(--text-sub) 45%, transparent);
  border-radius: 999px;
  border: 2px solid transparent;
  background-clip: content-box;
}

.providersList::-webkit-scrollbar-track,
.providersEditor::-webkit-scrollbar-track {
  background: transparent;
}

@media (max-width: 980px) {
  .providersHeader {
    padding-bottom: 14px;
  }

  .providersHeaderActions {
    margin-left: 0;
    width: 100%;
    justify-content: flex-start;
  }

  .providersSplit {
    grid-template-columns: 1fr;
    grid-template-rows: minmax(180px, 34%) minmax(0, 1fr);
  }

  .providersCatalogGrid {
    grid-template-columns: 1fr;
  }

  .providersBindGrid {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 720px) {
  .providersModal {
    width: min(98vw, 1160px);
    height: min(94vh, 920px);
  }

  .providersHeaderActions .headerMiniBtn {
    flex: 1 1 calc(50% - 8px);
  }
}
</style>
