<template>
  <router-view />
</template>

<script>
export default {
  name: 'App'
}
</script>

<style>
* {
  box-sizing: border-box;
}

body {
  margin: 0;
  font-family: "PingFang SC", "HarmonyOS Sans", "Helvetica Neue", Arial, sans-serif;
  background: #f7f8fa;
  color: #1f2933;
}

.button {
  background: linear-gradient(135deg, #2563eb, #4f46e5);
  color: #ffffff;
  border: none;
  padding: 0.75rem 1.5rem;
  border-radius: 8px;
  font-size: 0.95rem;
  cursor: pointer;
  transition: transform 0.15s ease, box-shadow 0.15s ease;
}

.button:hover {
  transform: translateY(-1px);
  box-shadow: 0 8px 20px rgba(79, 70, 229, 0.25);
}

.button:disabled {
  opacity: 0.6;
  cursor: default;
  box-shadow: none;
  transform: none;
}

.button--ghost {
  background: transparent;
  color: #2563eb;
  border: 1px solid rgba(37, 99, 235, 0.4);
}

.panel {
  background: #ffffff;
  border-radius: 12px;
  padding: 2rem;
  box-shadow: 0 10px 40px rgba(15, 23, 42, 0.05);
}

.panel--center {
  text-align: center;
  padding: 3rem 2rem;
}

.panel--highlight {
  display: flex;
  align-items: center;
}

.alert {
  padding: 1rem 1.5rem;
  border-radius: 8px;
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.alert--error {
  background: #fee2e2;
  color: #b91c1c;
  border: 1px solid #fca5a5;
}

.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.12s ease;
}

.fade-enter,
.fade-leave-to {
  opacity: 0;
}
</style>
