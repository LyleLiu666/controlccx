<script setup lang="ts">
import { computed } from "vue";
import type {
  AuthStatus,
  ProviderActiveSelection,
  ProviderProfile,
  ProviderSpeedTestResult,
} from "../types";

type SecretaryBackend = "auto" | "simple-http" | "claude" | "codex";
type SpeedTestTarget = "claude" | "codex" | "";

const props = defineProps<{
  open: boolean;
  loading: boolean;
  saving: boolean;
  error: string;
  storagePath: string;
  authStatus: AuthStatus | null;
  profiles: ProviderProfile[];
  active: ProviderActiveSelection;

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
  (e: "update:secretarySyncLive", value: boolean): void;
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
const secretarySyncLiveModel = computed({
  get: () => props.secretarySyncLive,
  set: (value: boolean) => emit("update:secretarySyncLive", value),
});
</script>

<template>
  <div v-if="open" class="modalOverlay" @click.self="emit('close')">
    <div class="modal toolsModal">
      <div class="modalHeader">
        <div class="modalTitle">Providers</div>
        <button type="button" class="headerMiniBtn" @click="emit('newProfile')">
          New
        </button>
        <button
          type="button"
          class="headerMiniBtn"
          @click="emit('refresh')"
          :disabled="loading || saving"
        >
          Refresh
        </button>
        <button
          type="button"
          class="headerMiniBtn"
          @click="emit('importLive')"
          :disabled="loading || saving"
        >
          Import live
        </button>
        <button
          type="button"
          class="headerMiniBtn"
          @click="emit('export', false)"
          :disabled="loading || saving"
        >
          Export
        </button>
        <button
          type="button"
          class="headerMiniBtn"
          @click="emit('export', true)"
          :disabled="loading || saving"
        >
          Export secrets
        </button>
        <button class="iconBtn" type="button" @click="emit('close')">✕</button>
      </div>

      <div class="modalBody toolsBody">
        <div v-if="storagePath" class="settingsMeta">
          Storage: <span class="mono">{{ storagePath }}</span>
        </div>
        <div v-if="authStatus?.warnings?.length" class="providersWarning">
          <div class="providersWarningTitle">Env overrides detected</div>
          <div class="tinyHint">
            Unset these environment variables and restart ControlCCX to make provider
            switches take effect.
          </div>
          <div class="providersWarningList mono">
            <div v-for="w in authStatus.warnings" :key="w">{{ w }}</div>
          </div>
        </div>
        <div v-if="error" class="modalError">{{ error }}</div>
        <div v-else-if="loading" class="loading">Loading...</div>
        <template v-else>
          <div class="toolsSplit">
            <div class="toolsList">
              <button
                v-for="p in profiles"
                :key="p.id"
                type="button"
                class="toolsItem"
                :class="{ active: p.id === editID }"
                @click="emit('selectProfile', p)"
              >
                <div class="mono">{{ p.name || p.id }}</div>
                <div class="providersBadges">
                  <span
                    class="providersBadge"
                    :class="{ on: active.claude === p.id }"
                    title="Claude"
                    >C</span
                  >
                  <span
                    class="providersBadge"
                    :class="{ on: active.codex === p.id }"
                    title="Codex"
                    >O</span
                  >
                  <span
                    class="providersBadge"
                    :class="{ on: active.secretary === p.id }"
                    title="Secretary"
                    >S</span
                  >
                </div>
              </button>
            </div>

            <div class="toolsEditor">
              <div class="toolsEditorGrid">
                <label class="full">
                  name
                  <input
                    v-model="editNameModel"
                    placeholder="Current"
                    autocomplete="off"
                  />
                </label>

                <details class="providerDetails" open>
                  <summary>Claude</summary>
                  <div class="toolsEditorGrid">
                    <label class="full">
                      base url
                      <input
                        v-model="claudeBaseURLModel"
                        placeholder="https://api.anthropic.com"
                        autocomplete="off"
                      />
                    </label>
                    <label class="full">
                      auth token (preferred)
                      <input
                        v-model="claudeAuthTokenModel"
                        type="password"
                        :placeholder="
                          claudeAuthTokenHint
                            ? `Leave blank to keep (${claudeAuthTokenHint})`
                            : 'Leave blank to keep'
                        "
                        autocomplete="off"
                      />
                    </label>
                    <label class="full">
                      api key
                      <input
                        v-model="claudeApiKeyModel"
                        type="password"
                        :placeholder="
                          claudeApiKeyHint
                            ? `Leave blank to keep (${claudeApiKeyHint})`
                            : 'Leave blank to keep'
                        "
                        autocomplete="off"
                      />
                    </label>
                    <label class="full">
                      model
                      <input
                        v-model="claudeModelModel"
                        placeholder="claude-3-7-sonnet"
                        autocomplete="off"
                      />
                    </label>
                    <label class="full">
                      small fast model
                      <input
                        v-model="claudeSmallFastModelModel"
                        placeholder="claude-3-5-haiku"
                        autocomplete="off"
                      />
                    </label>
                    <label class="settingsToggleRow">
                      <input type="checkbox" v-model="claudeSyncLiveModel" />
                      <span>Sync live config on activate</span>
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
                          >Testing...</span
                        >
                        <span v-else>Speed test</span>
                      </button>
                      <div
                        v-if="claudeSpeedTest"
                        class="speedTestResult mono"
                        :class="{ ok: claudeSpeedTest.ok, bad: !claudeSpeedTest.ok }"
                      >
                        <span>{{ claudeSpeedTest.ok ? "ok" : "fail" }}</span>
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
                    <div class="providerActions">
                      <button
                        type="button"
                        class="primary"
                        @click="emit('activate', 'claude')"
                        :disabled="saving || !editID.trim()"
                      >
                        Activate Claude
                      </button>
                    </div>
                  </div>
                </details>

                <details class="providerDetails">
                  <summary>Codex</summary>
                  <div class="toolsEditorGrid">
                    <label class="full">
                      base url (optional)
                      <input
                        v-model="codexBaseURLModel"
                        placeholder="https://api.openai.com"
                        autocomplete="off"
                      />
                    </label>
                    <label class="full">
                      api key
                      <input
                        v-model="codexApiKeyModel"
                        type="password"
                        :placeholder="
                          codexApiKeyHint
                            ? `Leave blank to keep (${codexApiKeyHint})`
                            : 'Leave blank to keep'
                        "
                        autocomplete="off"
                      />
                    </label>
                    <label class="full">
                      model
                      <input
                        v-model="codexModelModel"
                        placeholder="gpt-5.2"
                        autocomplete="off"
                      />
                    </label>
                    <label class="full">
                      reasoning effort
                      <select v-model="codexReasoningEffortModel">
                        <option value="">(default)</option>
                        <option value="low">low</option>
                        <option value="medium">medium</option>
                        <option value="high">high</option>
                        <option value="xhigh">xhigh</option>
                      </select>
                    </label>
                    <label class="settingsToggleRow">
                      <input type="checkbox" v-model="codexSyncLiveModel" />
                      <span>Sync live config on activate</span>
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
                          >Testing...</span
                        >
                        <span v-else>Speed test</span>
                      </button>
                      <div
                        v-if="codexSpeedTest"
                        class="speedTestResult mono"
                        :class="{ ok: codexSpeedTest.ok, bad: !codexSpeedTest.ok }"
                      >
                        <span>{{ codexSpeedTest.ok ? "ok" : "fail" }}</span>
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
                    <div class="providerActions">
                      <button
                        type="button"
                        class="primary"
                        @click="emit('activate', 'codex')"
                        :disabled="saving || !editID.trim()"
                      >
                        Activate Codex
                      </button>
                    </div>
                  </div>
                </details>

                <details class="providerDetails">
                  <summary>Secretary</summary>
                  <div class="toolsEditorGrid">
                    <label class="full">
                      backend
                      <select v-model="secretaryBackendModel">
                        <option value="auto">auto</option>
                        <option value="simple-http">simple-http</option>
                        <option value="claude">claude</option>
                        <option value="codex">codex</option>
                      </select>
                    </label>
                    <label class="settingsToggleRow">
                      <input type="checkbox" v-model="secretarySyncLiveModel" disabled />
                      <span>Sync live config on activate (n/a)</span>
                    </label>
                    <div class="providerActions">
                      <button
                        type="button"
                        class="primary"
                        @click="emit('activate', 'secretary')"
                        :disabled="saving || !editID.trim()"
                      >
                        Activate Secretary
                      </button>
                    </div>
                  </div>
                </details>
              </div>
            </div>
          </div>
        </template>
      </div>

      <div class="modalFooter">
        <button type="button" @click="emit('close')">Close</button>
        <button type="button" @click="emit('delete')" :disabled="saving || !editID.trim()">
          Delete
        </button>
        <button
          type="button"
          class="primary"
          @click="emit('save')"
          :disabled="saving || !editName.trim()"
        >
          {{ saving ? "Saving..." : "Save" }}
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.providersWarning {
  border: 1px solid rgba(245, 158, 11, 0.35);
  background: rgba(245, 158, 11, 0.08);
  border-radius: 14px;
  padding: 12px;
  margin-bottom: 10px;
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

.providersBadges {
  display: flex;
  gap: 6px;
  margin-top: 6px;
}

.providersBadge {
  width: 22px;
  height: 22px;
  display: inline-grid;
  place-items: center;
  border-radius: 999px;
  border: 1px solid var(--border-color);
  background: transparent;
  color: var(--text-muted);
  font-weight: 800;
  font-size: 12px;
}

.providersBadge.on {
  border-color: rgba(20, 184, 166, 0.45);
  background: rgba(20, 184, 166, 0.12);
  color: var(--text-main);
}

.providerDetails {
  border: 1px solid var(--border-color);
  border-radius: 14px;
  padding: 10px 12px;
  background: var(--bg-subtle);
}

.providerDetails > summary {
  cursor: pointer;
  list-style: none;
  font-weight: 800;
  color: var(--text-main);
}

.providerDetails > summary::-webkit-details-marker {
  display: none;
}

.providerDetails[open] > summary {
  margin-bottom: 10px;
}

.providerActions {
  display: flex;
  justify-content: flex-end;
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
</style>
