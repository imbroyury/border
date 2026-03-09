import { createRouter, createWebHistory } from 'vue-router'
import DashboardView from './views/DashboardView.vue'
import ZoneDetailView from './views/ZoneDetailView.vue'
import VehicleSearchView from './views/VehicleSearchView.vue'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', name: 'dashboard', component: DashboardView },
    { path: '/zone/:id', name: 'zone-detail', component: ZoneDetailView, props: true },
    { path: '/vehicles', name: 'vehicle-search', component: VehicleSearchView },
  ],
})

export default router
