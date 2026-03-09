<template>
  <div class="app">
    <header class="app-header">
      <router-link to="/" class="app-title">Border Queue Monitor</router-link>
    </header>
    <main class="app-main">
      <router-view />
    </main>
  </div>
</template>

<style scoped>
.app-header {
  background: #1a1a2e;
  color: #e0e0e0;
  padding: 1rem 2rem;
  border-bottom: 2px solid #16213e;
}

.app-title {
  font-size: 1.4rem;
  font-weight: 700;
  color: #e0e0e0;
  text-decoration: none;
}

.app-main {
  max-width: 1200px;
  margin: 0 auto;
  padding: 1.5rem;
}
</style>
