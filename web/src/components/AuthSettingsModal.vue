<script setup lang="ts">
import { computed } from "vue";
import type { AuthStatus, ToolStatus } from "../types";

type StoredAuthKey =
  | "anthropic_base_url"
  | "anthropic_api_key"
  | "anthropic_auth_token"
  | "anthropic_model"
  | "anthropic_small_fast_model"
  | "openai_api_key"
  | "codex_model"
  | "codex_reasoning_effort";

const props = defineProps<{
  open: boolean;
  saving: boolean;
  error: string;
  storagePath: string;
  authStatus: AuthStatus | null;
  toolsStatus: ToolStatus[] | null;
  autoDeliveryForeman: boolean;
  anthropicBaseURL: string;
  anthropicApiKey: string;
  anthropicAuthToken: string;
  anthropicModel: string;
  anthropicSmallFastModel: string;
  openAIApiKey: string;
  codexModel: string;
  codexReasoningEffort: string;
}>();

const emit = defineEmits<{
  (e: "close"): void;
  (e: "openTools"): void;
  (e: "save"): void;
  (e: "clearStored", key: StoredAuthKey): void;
  (e: "update:autoDeliveryForeman", value: boolean): void;
  (e: "update:anthropicBaseURL", value: string): void;
  (e: "update:anthropicApiKey", value: string): void;
  (e: "update:anthropicAuthToken", value: string): void;
  (e: "update:anthropicModel", value: string): void;
  (e: "update:anthropicSmallFastModel", value: string): void;
  (e: "update:openAIApiKey", value: string): void;
  (e: "update:codexModel", value: string): void;
  (e: "update:codexReasoningEffort", value: string): void;
}>();

const autoDeliveryForemanModel = computed({
  get: () => props.autoDeliveryForeman,
  set: (value: boolean) => emit("update:autoDeliveryForeman", value),
});
const anthropicBaseURLModel = computed({
  get: () => props.anthropicBaseURL,
  set: (value: string) => emit("update:anthropicBaseURL", value),
});
const anthropicApiKeyModel = computed({
  get: () => props.anthropicApiKey,
  set: (value: string) => emit("update:anthropicApiKey", value),
});
const anthropicAuthTokenModel = computed({
  get: () => props.anthropicAuthToken,
  set: (value: string) => emit("update:anthropicAuthToken", value),
});
const anthropicModelModel = computed({
  get: () => props.anthropicModel,
  set: (value: string) => emit("update:anthropicModel", value),
});
const anthropicSmallFastModelModel = computed({
  get: () => props.anthropicSmallFastModel,
  set: (value: string) => emit("update:anthropicSmallFastModel", value),
});
const openAIApiKeyModel = computed({
  get: () => props.openAIApiKey,
  set: (value: string) => emit("update:openAIApiKey", value),
});
const codexModelModel = computed({
  get: () => props.codexModel,
  set: (value: string) => emit("update:codexModel", value),
});
const codexReasoningEffortModel = computed({
  get: () => props.codexReasoningEffort,
  set: (value: string) => emit("update:codexReasoningEffort", value),
});

const claudeToolStatus = computed<ToolStatus | null>(
  () => props.toolsStatus?.find((t) => t.id === "claude-code") ?? null,
);
const codexToolStatus = computed<ToolStatus | null>(
  () => props.toolsStatus?.find((t) => t.id === "codex") ?? null,
);
const showCliInstallGuide = computed<boolean>(() => {
  if (!claudeToolStatus.value || !codexToolStatus.value) return false;
  return !claudeToolStatus.value.available && !codexToolStatus.value.available;
});
</script>

<template>
  <div v-if="open" class="modalOverlay" @click.self="emit('close')">
    <div class="modal settingsModal">
      <div class="modalHeader">
        <div class="modalTitle">Auth Settings</div>
        <button type="button" class="headerMiniBtn" @click="emit('openTools')">
          Tools
        </button>
        <button class="iconBtn" type="button" @click="emit('close')">✕</button>
      </div>

      <div class="modalBody settingsBody">
        <div class="settingsMeta" v-if="storagePath">
          Storage: <span class="mono">{{ storagePath }}</span>
        </div>

        <div v-if="error" class="modalError">{{ error }}</div>

        <div v-if="showCliInstallGuide" class="settingsSection">
          <div class="settingsSectionTitle">快速开始</div>
          <div class="tinyHint">
            未检测到可用的 Claude Code / Codex 命令。你需要先安装 Claude Code（推荐）或在
            Tools 里配置可执行文件路径。
          </div>
          <ol class="setupSteps">
            <li>
              安装
              <a
                href="https://nodejs.org/en/download/"
                target="_blank"
                rel="noopener noreferrer"
                >Node.js 18 或更新版本环境</a
              >。
            </li>
            <li>
              Windows 用户需安装
              <a
                href="https://git-scm.com/download/win"
                target="_blank"
                rel="noopener noreferrer"
                >Git for Windows</a
              >。
            </li>
            <li>
              在命令行界面，执行以下命令安装 Claude Code：<br />
              <span class="mono">npm install -g @anthropic-ai/claude-code</span>
            </li>
            <li>
              安装结束后，执行以下命令查看安装结果：<br />
              <span class="mono">claude --version</span>
            </li>
          </ol>
          <div class="setupProvider">
            邀请注册火山作为 provider：
            <a
              href="https://volcengine.com/L/N2h_TKPIsvA/"
              target="_blank"
              rel="noopener noreferrer"
              >volcengine.com</a
            >
            ，邀请码：<span class="mono">RTGWR7T3</span>
          </div>
        </div>

        <div class="settingsSection">
          <div class="settingsSectionTitle">Automation</div>
          <label class="settingsToggleRow">
            <input type="checkbox" v-model="autoDeliveryForemanModel" />
            <span>Auto Delivery Foreman</span>
          </label>
          <div class="tinyHint">
            When a run finishes, send an automatic “delivery check” message to the
            Secretary (no auto focus).
          </div>
        </div>

        <div class="settingsSection">
          <div class="settingsSectionTitle">Claude Code</div>
          <div class="kv">
            <span class="k">ANTHROPIC_BASE_URL</span>
            <span class="mono"
              >{{ authStatus?.claude.base_url.effective }}
              {{ authStatus?.claude.base_url.masked }}</span
            >
          </div>
          <div class="kv">
            <span class="k">ANTHROPIC_API_KEY</span>
            <span class="mono"
              >{{ authStatus?.claude.api_key.effective }}
              {{ authStatus?.claude.api_key.masked }}</span
            >
          </div>
          <div class="kv">
            <span class="k">ANTHROPIC_AUTH_TOKEN</span>
            <span class="mono"
              >{{ authStatus?.claude.auth_token.effective }}
              {{ authStatus?.claude.auth_token.masked }}</span
            >
          </div>
          <div class="kv">
            <span class="k">ANTHROPIC_MODEL</span>
            <span class="mono"
              >{{ authStatus?.claude.model.effective }}
              {{ authStatus?.claude.model.masked }}</span
            >
          </div>
          <div class="kv">
            <span class="k">ANTHROPIC_SMALL_FAST_MODEL</span>
            <span class="mono"
              >{{ authStatus?.claude.small_fast_model.effective }}
              {{ authStatus?.claude.small_fast_model.masked }}</span
            >
          </div>

          <label class="full">
            Store ANTHROPIC_BASE_URL
            <div class="secretRow">
              <input
                v-model="anthropicBaseURLModel"
                placeholder="https://..."
                autocomplete="off"
              />
              <button
                type="button"
                @click="emit('clearStored', 'anthropic_base_url')"
                :disabled="saving"
              >
                Clear stored
              </button>
            </div>
          </label>
          <label class="full">
            Store ANTHROPIC_API_KEY
            <div class="secretRow">
              <input
                v-model="anthropicApiKeyModel"
                type="password"
                placeholder="Paste key…"
                autocomplete="off"
              />
              <button
                type="button"
                @click="emit('clearStored', 'anthropic_api_key')"
                :disabled="saving"
              >
                Clear stored
              </button>
            </div>
          </label>
          <label class="full">
            Store ANTHROPIC_AUTH_TOKEN
            <div class="secretRow">
              <input
                v-model="anthropicAuthTokenModel"
                type="password"
                placeholder="Paste token…"
                autocomplete="off"
              />
              <button
                type="button"
                @click="emit('clearStored', 'anthropic_auth_token')"
                :disabled="saving"
              >
                Clear stored
              </button>
            </div>
          </label>
          <label class="full">
            Store ANTHROPIC_MODEL
            <div class="secretRow">
              <input
                v-model="anthropicModelModel"
                placeholder="model name…"
                autocomplete="off"
              />
              <button
                type="button"
                @click="emit('clearStored', 'anthropic_model')"
                :disabled="saving"
              >
                Clear stored
              </button>
            </div>
          </label>
          <label class="full">
            Store ANTHROPIC_SMALL_FAST_MODEL
            <div class="secretRow">
              <input
                v-model="anthropicSmallFastModelModel"
                placeholder="model name…"
                autocomplete="off"
              />
              <button
                type="button"
                @click="emit('clearStored', 'anthropic_small_fast_model')"
                :disabled="saving"
              >
                Clear stored
              </button>
            </div>
          </label>

          <div class="settingsHelp">
            如果你使用 Claude Code 订阅登录模式，也可以在终端运行一次
            <span class="mono">claude /login</span>。
          </div>
        </div>

        <div class="settingsSection">
          <div class="settingsSectionTitle">Codex</div>
          <div class="kv">
            <span class="k">OPENAI_API_KEY</span>
            <span class="mono"
              >{{ authStatus?.codex.api_key.effective }}
              {{ authStatus?.codex.api_key.masked }}</span
            >
          </div>
          <div class="kv">
            <span class="k">MODEL</span>
            <span class="mono"
              >{{ authStatus?.codex.model.effective }}
              {{ authStatus?.codex.model.masked }}</span
            >
          </div>
          <div class="kv">
            <span class="k">REASONING</span>
            <span class="mono"
              >{{ authStatus?.codex.reasoning_effort.effective }}
              {{ authStatus?.codex.reasoning_effort.masked }}</span
            >
          </div>
          <label class="full">
            Store OPENAI_API_KEY
            <div class="secretRow">
              <input
                v-model="openAIApiKeyModel"
                type="password"
                placeholder="Paste key…"
                autocomplete="off"
              />
              <button
                type="button"
                @click="emit('clearStored', 'openai_api_key')"
                :disabled="saving"
              >
                Clear stored
              </button>
            </div>
          </label>
          <label class="full">
            Set model (default gpt-5.2)
            <div class="secretRow">
              <input
                v-model="codexModelModel"
                placeholder="gpt-5.2"
                autocomplete="off"
              />
              <button
                type="button"
                @click="emit('clearStored', 'codex_model')"
                :disabled="saving"
              >
                Clear stored
              </button>
            </div>
          </label>
          <label class="full">
            Set reasoning effort (default xhigh)
            <div class="secretRow">
              <select v-model="codexReasoningEffortModel">
                <option value="">(keep)</option>
                <option value="low">low</option>
                <option value="medium">medium</option>
                <option value="high">high</option>
                <option value="xhigh">xhigh</option>
              </select>
              <button
                type="button"
                @click="emit('clearStored', 'codex_reasoning_effort')"
                :disabled="saving"
              >
                Clear stored
              </button>
            </div>
          </label>
        </div>
      </div>

      <div class="modalFooter">
        <button type="button" @click="emit('close')">Close</button>
        <button
          type="button"
          class="primary"
          @click="emit('save')"
          :disabled="saving"
        >
          {{ saving ? "Saving..." : "Save" }}
        </button>
      </div>
    </div>
  </div>
</template>
