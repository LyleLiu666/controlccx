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
  notice?: string;
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
  (e: "openProviders"): void;
  (e: "openAudit"): void;
  (e: "importEnv", target: "claude" | "codex" | "all"): void;
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

const canImportClaudeEnv = computed<boolean>(() => {
  const st = props.authStatus?.claude;
  if (!st) return false;
  return [
    st.base_url,
    st.api_key,
    st.auth_token,
    st.model,
    st.small_fast_model,
  ].some((f) => f?.effective === "env");
});

const canImportCodexEnv = computed<boolean>(() => {
  const st = props.authStatus?.codex;
  if (!st) return false;
  return st.api_key?.effective === "env";
});

function formatAuthEffective(v: string | undefined | null): string {
  const s = String(v ?? "").trim().toLowerCase();
  switch (s) {
    case "env":
      return "环境变量";
    case "stored":
      return "已保存";
    case "live":
      return "CLI";
    case "default":
      return "默认";
    case "none":
    case "":
      return "无";
    default:
      return s;
  }
}
</script>

<template>
  <div v-if="open" class="modalOverlay" @click.self="emit('close')">
    <div class="modal settingsModal">
      <div class="modalHeader">
        <div class="modalTitle">认证设置</div>
        <button type="button" class="headerMiniBtn" @click="emit('openTools')">
          工具
        </button>
        <button type="button" class="headerMiniBtn" @click="emit('openProviders')">
          提供方
        </button>
        <button type="button" class="headerMiniBtn" @click="emit('openAudit')">
          审计日志
        </button>
        <button class="iconBtn" type="button" @click="emit('close')">✕</button>
      </div>

      <div class="modalBody settingsBody">
        <div class="settingsMeta" v-if="storagePath">
          存储位置: <span class="mono">{{ storagePath }}</span>
        </div>

        <div v-if="error" class="modalError">{{ error }}</div>
        <div v-if="notice" class="modalNotice">{{ notice }}</div>

        <div class="setupHint">
          <div><strong>不知道怎么录入新的提供方？</strong></div>
          <div class="tinyHint">
            工具 = 本地命令（Claude Code / Codex），提供方 = 远端 API。你不需要把工具“绑定”到提供方：
            只要在提供方页里把令牌/模型保存并启用，新建 run 时会自动使用。
          </div>
          <ol class="setupSteps">
            <li>
              录入新的提供方：点击右上角 <span class="mono">提供方</span>，在左侧进入 <span class="mono">总览</span>，
              再选择 Claude Code / Codex / 秘书。
            </li>
            <li>
              在对应页面填写 <span class="mono">授权</span> 与 <span class="mono">模型</span>，然后点击 <span class="mono">保存并启用</span>（立即生效，只影响后续新 run）。
            </li>
            <li>
              回到这里确认状态变成 <span class="mono">已保存</span>（或 <span class="mono">环境变量</span>）。
            </li>
          </ol>
          <div class="setupActions">
            <button type="button" class="primary" @click="emit('openProviders')">
              打开提供方
            </button>
            <button type="button" @click="emit('openTools')">
              打开工具
            </button>
          </div>
        </div>

        <div v-if="showCliInstallGuide" class="settingsSection">
          <div class="settingsSectionTitle">快速开始</div>
          <div class="tinyHint">
            未检测到可用的 Claude Code / Codex 命令。你需要先安装 Claude Code（推荐）或在
            工具中配置可执行文件路径。
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
          <div class="settingsSectionTitle">自动化</div>
          <label class="settingsToggleRow">
            <input type="checkbox" v-model="autoDeliveryForemanModel" />
            <span>自动交付前哨</span>
          </label>
          <div class="tinyHint">
            当 run 结束时，自动向秘书发送一条“交付检查”消息（不会自动抢焦点）。
          </div>
        </div>

        <div class="settingsSection">
          <div class="settingsSectionTitleRow">
            <div class="settingsSectionTitle">Claude Code</div>
            <button
              v-if="canImportClaudeEnv"
              type="button"
              class="settingsSectionActionBtn"
              @click="emit('importEnv', 'claude')"
              :disabled="saving"
            >
              保存环境变量到本地
            </button>
          </div>
          <div class="kv">
            <span class="k">ANTHROPIC_BASE_URL</span>
            <span class="mono"
              >{{ formatAuthEffective(authStatus?.claude.base_url.effective) }}
              {{ authStatus?.claude.base_url.masked }}</span
            >
          </div>
          <div class="kv">
            <span class="k">ANTHROPIC_API_KEY</span>
            <span class="mono"
              >{{ formatAuthEffective(authStatus?.claude.api_key.effective) }}
              {{ authStatus?.claude.api_key.masked }}</span
            >
          </div>
          <div class="kv">
            <span class="k">ANTHROPIC_AUTH_TOKEN</span>
            <span class="mono"
              >{{ formatAuthEffective(authStatus?.claude.auth_token.effective) }}
              {{ authStatus?.claude.auth_token.masked }}</span
            >
          </div>
          <div class="kv">
            <span class="k">ANTHROPIC_MODEL</span>
            <span class="mono"
              >{{ formatAuthEffective(authStatus?.claude.model.effective) }}
              {{ authStatus?.claude.model.masked }}</span
            >
          </div>
          <div class="kv">
            <span class="k">ANTHROPIC_SMALL_FAST_MODEL</span>
            <span class="mono"
              >{{ formatAuthEffective(authStatus?.claude.small_fast_model.effective) }}
              {{ authStatus?.claude.small_fast_model.masked }}</span
            >
          </div>

          <label class="full">
            保存 ANTHROPIC_BASE_URL
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
                清除已保存
              </button>
            </div>
          </label>
          <label class="full">
            保存 ANTHROPIC_API_KEY
            <div class="secretRow">
              <input
                v-model="anthropicApiKeyModel"
                type="password"
                placeholder="粘贴 key…"
                autocomplete="off"
              />
              <button
                type="button"
                @click="emit('clearStored', 'anthropic_api_key')"
                :disabled="saving"
              >
                清除已保存
              </button>
            </div>
          </label>
          <label class="full">
            保存 ANTHROPIC_AUTH_TOKEN
            <div class="secretRow">
              <input
                v-model="anthropicAuthTokenModel"
                type="password"
                placeholder="粘贴 token…"
                autocomplete="off"
              />
              <button
                type="button"
                @click="emit('clearStored', 'anthropic_auth_token')"
                :disabled="saving"
              >
                清除已保存
              </button>
            </div>
          </label>
          <label class="full">
            保存 ANTHROPIC_MODEL
            <div class="secretRow">
              <input
                v-model="anthropicModelModel"
                placeholder="模型名…"
                autocomplete="off"
              />
              <button
                type="button"
                @click="emit('clearStored', 'anthropic_model')"
                :disabled="saving"
              >
                清除已保存
              </button>
            </div>
          </label>
          <label class="full">
            保存 ANTHROPIC_SMALL_FAST_MODEL
            <div class="secretRow">
              <input
                v-model="anthropicSmallFastModelModel"
                placeholder="模型名…"
                autocomplete="off"
              />
              <button
                type="button"
                @click="emit('clearStored', 'anthropic_small_fast_model')"
                :disabled="saving"
              >
                清除已保存
              </button>
            </div>
          </label>

          <div class="settingsHelp">
            如果你使用 Claude Code 订阅登录模式，也可以在终端运行一次
            <span class="mono">claude /login</span>。
          </div>
        </div>

        <div class="settingsSection">
          <div class="settingsSectionTitleRow">
            <div class="settingsSectionTitle">Codex</div>
            <button
              v-if="canImportCodexEnv"
              type="button"
              class="settingsSectionActionBtn"
              @click="emit('importEnv', 'codex')"
              :disabled="saving"
            >
              保存环境变量到本地
            </button>
          </div>
          <div class="kv">
            <span class="k">OPENAI_API_KEY</span>
            <span class="mono"
              >{{ formatAuthEffective(authStatus?.codex.api_key.effective) }}
              {{ authStatus?.codex.api_key.masked }}</span
            >
          </div>
          <div class="kv">
            <span class="k">MODEL</span>
            <span class="mono"
              >{{ formatAuthEffective(authStatus?.codex.model.effective) }}
              {{ authStatus?.codex.model.masked }}</span
            >
          </div>
          <div class="kv">
            <span class="k">REASONING</span>
            <span class="mono"
              >{{ formatAuthEffective(authStatus?.codex.reasoning_effort.effective) }}
              {{ authStatus?.codex.reasoning_effort.masked }}</span
            >
          </div>
          <label class="full">
            保存 OPENAI_API_KEY
            <div class="secretRow">
              <input
                v-model="openAIApiKeyModel"
                type="password"
                placeholder="粘贴 key…"
                autocomplete="off"
              />
              <button
                type="button"
                @click="emit('clearStored', 'openai_api_key')"
                :disabled="saving"
              >
                清除已保存
              </button>
            </div>
          </label>
          <label class="full">
            模型（默认 gpt-5.2）
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
                清除已保存
              </button>
            </div>
          </label>
          <label class="full">
            推理强度（默认 xhigh）
            <div class="secretRow">
              <select v-model="codexReasoningEffortModel">
                <option value="">(保持)</option>
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
                清除已保存
              </button>
            </div>
          </label>
        </div>
      </div>

      <div class="modalFooter">
        <button type="button" @click="emit('close')">关闭</button>
        <button
          type="button"
          class="primary"
          @click="emit('save')"
          :disabled="saving"
        >
          {{ saving ? "保存中..." : "保存" }}
        </button>
      </div>
    </div>
  </div>
</template>
