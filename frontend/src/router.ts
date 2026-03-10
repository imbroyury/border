import { createRouter, createWebHistory } from 'vue-router'
import DashboardView from './views/DashboardView.vue'
import ZoneDetailView from './views/ZoneDetailView.vue'
import VehicleListView from './views/VehicleListView.vue'
import VehicleHistoryView from './views/VehicleHistoryView.vue'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', name: 'dashboard', component: DashboardView },
    { path: '/zone/:id', name: 'zone-detail', component: ZoneDetailView, props: true },
    { path: '/vehicles', name: 'vehicle-list', component: VehicleListView },
    { path: '/vehicles/:regNumber', name: 'vehicle-history', component: VehicleHistoryView, props: true },
  ],
})

export default router
