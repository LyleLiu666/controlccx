<script setup lang="ts">
import { computed } from "vue";

type FileNode = {
  name: string;
  path: string;
  kind: "dir" | "file";
  size?: number;
  expanded?: boolean;
  children?: FileNode[];
  loading?: boolean;
};

type VisibleNode = { node: FileNode; depth: number };

const props = defineProps<{
  root: FileNode | null;
  loading: boolean;
  error: string;
  notice: string;
  sidebarWidth: number;
  resizing: boolean;
  visibleNodes: VisibleNode[];
  selectedPath: string;
  selectedKind: "" | "dir" | "file";
  view: "preview" | "edit";
  dirty: boolean;
  fileSize: number;
  fileTruncated: boolean;
  fileContent: string;
  fileError: string;
  fileLoading: boolean;
  saving: boolean;
  isMarkdown: boolean;
  previewHtml: string;
  codeHtml: string;
  normalizePathForCompare: (p: string) => string;
}>();

const emit = defineEmits<{
  (e: "back"): void;
  (e: "refreshRoot"): void;
  (e: "newFile"): void;
  (e: "newFolder"): void;
  (e: "deleteSelected"): void;
  (e: "refreshDir"): void;
  (e: "nodeClick", node: FileNode): void;
  (e: "startResize", ev: MouseEvent): void;
  (e: "copy", text: string): void;
  (e: "save"): void;
  (e: "markdownClick", ev: MouseEvent): void;
  (e: "update:view", value: "preview" | "edit"): void;
  (e: "update:fileContent", value: string): void;
}>();

const viewModel = computed({
  get: () => props.view,
  set: (value: "preview" | "edit") => emit("update:view", value),
});
const fileContentModel = computed({
  get: () => props.fileContent,
  set: (value: string) => emit("update:fileContent", value),
});

function isActiveNode(path: string): boolean {
  return (
    props.normalizePathForCompare(path) ===
    props.normalizePathForCompare(props.selectedPath)
  );
}
</script>

<template>
  <section class="panel filesPagePanel">
    <h2>
      Files
      <span class="h2Spacer"></span>
      <button
        type="button"
        class="h2Btn"
        @click="emit('refreshRoot')"
        :disabled="loading || !root"
      >
        Refresh
      </button>
      <button type="button" class="h2Btn" @click="emit('back')">Back</button>
    </h2>

    <div class="filesPageBody">
      <div v-if="error" class="modalError">
        {{ error }}
      </div>
      <div v-else-if="loading" class="loading">Loading...</div>
      <template v-else>
        <div class="filesTopRow">
          <div class="mono filesRootPath" :title="root?.path">
            {{ root?.path }}
          </div>
          <div v-if="notice" class="tinyHint">{{ notice }}</div>
        </div>

        <div class="filesSplit" :style="{ '--sidebar-width': sidebarWidth + 'px' }">
          <div class="filesTreePane">
            <div class="filesTreeActions">
              <button type="button" @click="emit('newFile')" :disabled="!root">
                New file
              </button>
              <button type="button" @click="emit('newFolder')" :disabled="!root">
                New folder
              </button>
              <button
                type="button"
                @click="emit('deleteSelected')"
                :disabled="!selectedPath || selectedPath === root?.path"
              >
                Delete
              </button>
              <button type="button" @click="emit('refreshDir')" :disabled="!root">
                Refresh
              </button>
            </div>

            <div class="filesTreeList">
              <button
                v-for="v in visibleNodes"
                :key="v.node.path"
                type="button"
                class="filesNode"
                :class="{ active: isActiveNode(v.node.path) }"
                :style="{ paddingLeft: `${12 + v.depth * 14}px` }"
                @click="emit('nodeClick', v.node)"
              >
                <span class="filesNodeTwisty">{{
                  v.node.kind === "dir" ? (v.node.expanded ? "▼" : "▶") : ""
                }}</span>
                <span class="filesNodeIcon">{{
                  v.node.kind === "dir" ? "📁" : "📄"
                }}</span>
                <span class="filesNodeName">{{ v.node.name }}</span>
                <span v-if="v.node.kind === 'file'" class="filesNodeMeta mono">{{
                  v.node.size ?? 0
                }}</span>
                <span v-if="v.node.loading" class="filesNodeMeta tinyHint">…</span>
              </button>
              <div v-if="!visibleNodes.length" class="empty">Empty folder</div>
            </div>
          </div>

          <div
            class="filesResizer"
            @mousedown="emit('startResize', $event)"
            :class="{ resizing }"
          ></div>

          <div class="filesEditorPane">
            <div v-if="selectedKind !== 'file'" class="empty">
              {{
                selectedKind === "dir"
                  ? "Select a file to preview/edit."
                  : "Select a file."
              }}
            </div>
            <template v-else>
              <div class="filesEditorHeader">
                <div class="mono filesEditorPath" :title="selectedPath">
                  {{ selectedPath }}
                </div>
                <span class="tinyHint mono">{{ fileSize }} bytes</span>
                <span v-if="fileTruncated" class="pill warn">truncated</span>
                <div class="tabSpacer"></div>
                <button
                  type="button"
                  @click="emit('copy', fileContent)"
                  :disabled="!fileContent"
                >
                  Copy
                </button>
                <button
                  type="button"
                  class="primary"
                  @click="emit('save')"
                  :disabled="saving || !dirty || fileTruncated"
                >
                  {{ saving ? "Saving..." : "Save" }}
                </button>
              </div>

              <div class="outputTabs">
                <button
                  type="button"
                  class="tabBtn"
                  :class="{ active: viewModel === 'preview' }"
                  @click="viewModel = 'preview'"
                >
                  Preview
                </button>
                <button
                  type="button"
                  class="tabBtn"
                  :class="{ active: viewModel === 'edit' }"
                  @click="viewModel = 'edit'"
                >
                  Edit
                </button>
                <div class="tabSpacer"></div>
                <div v-if="dirty" class="tinyHint">unsaved</div>
              </div>

              <div v-if="fileError" class="modalError">
                {{ fileError }}
              </div>
              <div v-else-if="fileLoading" class="loading">Loading...</div>
              <template v-else>
                <div v-if="viewModel === 'edit'" class="filesEditorEdit">
                  <textarea
                    v-model="fileContentModel"
                    rows="18"
                    spellcheck="false"
                  ></textarea>
                </div>
                <template v-else>
                  <div
                    v-if="isMarkdown"
                    class="resultBox markdown filePreviewBox"
                    v-html="previewHtml"
                    @click="emit('markdownClick', $event)"
                  ></div>

                  <div v-else class="resultBox fileCodeBox">
                    <pre class="hljs"><code v-html="codeHtml"></code></pre>
                  </div>
                </template>
              </template>
            </template>
          </div>
        </div>
      </template>
    </div>
  </section>
</template>

