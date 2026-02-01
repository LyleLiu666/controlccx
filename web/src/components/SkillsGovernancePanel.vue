<script setup lang="ts">
const emit = defineEmits<{
  (e: "open"): void;
}>();
</script>

<template>
  <div class="skillsGovLauncherCard">
    <div class="skillsGovLauncherHeader">
      <div class="skillsGovLauncherTitle">导入 / 安装 / 同步</div>
      <button type="button" class="primary" @click="emit('open')">
        打开
      </button>
    </div>
    <div class="tinyHint">
      把技能纳入来源库后，再在右侧列表启用到目标工具。
    </div>
  </div>
</template>

