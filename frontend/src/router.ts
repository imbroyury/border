import { createRouter, createWebHistory } from 'vue-router'
import DashboardView from './views/DashboardView.vue'
import ZoneDetailView from './views/ZoneDetailView.vue'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', name: 'dashboard', component: DashboardView },
    { path: '/zone/:id', name: 'zone-detail', component: ZoneDetailView, props: true },
  ],
})

export default router
