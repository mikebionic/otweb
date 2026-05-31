import { useQuery } from '@tanstack/react-query'
import { Grid, Card, CardContent, Typography, Box, Chip, LinearProgress, Divider } from '@mui/material'
import { Inventory2, Category, Sync, CloudUpload, Translate, CheckCircle } from '@mui/icons-material'
import api from '../api/client'
import type { DashboardStats, SyncJob } from '../types'

function StatCard({ label, value, icon, color }: { label: string; value: number | string; icon: React.ReactNode; color: string }) {
  return (
    <Card>
      <CardContent sx={{ display: 'flex', alignItems: 'center', gap: 2, py: '16px !important' }}>
        <Box sx={{ color, fontSize: 32, lineHeight: 1, opacity: 0.8 }}>{icon}</Box>
        <Box>
          <Typography sx={{ fontSize: 11, textTransform: 'uppercase', letterSpacing: '0.06em', color: '#6c757d', mb: '4px' }}>
            {label}
          </Typography>
          <Typography sx={{ fontSize: 26, fontWeight: 700, color: '#212529', lineHeight: 1 }}>
            {value}
          </Typography>
        </Box>
      </CardContent>
    </Card>
  )
}

function jobColor(s: string): any {
  return s === 'done' ? 'success' : s === 'running' ? 'info' : s === 'error' ? 'error' : 'default'
}

function ts(n: number) {
  if (!n) return '-'
  return new Date(n * 1000).toLocaleString('ru-RU', { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' })
}

export default function Dashboard() {
  const { data, isLoading } = useQuery({
    queryKey: ['dashboard'],
    queryFn: () => api.get('/dashboard').then(r => r.data.data as { stats: DashboardStats; jobs: SyncJob[] }),
    refetchInterval: 15_000,
  })

  if (isLoading) return <LinearProgress />
  const stats = data?.stats ?? {} as DashboardStats
  const jobs = data?.jobs ?? []

  return (
    <Box>
      <Typography sx={{ fontSize: 20, fontWeight: 700, mb: 3 }}>Dashboard</Typography>

      <Grid container spacing={2} sx={{ mb: 3 }}>
        <Grid size={{ xs: 12, sm: 6, md: 4 }}>
          <StatCard label="Товаров в БД" value={stats.TotalProducts ?? 0} icon={<Inventory2 fontSize="inherit" />} color="#6366f1" />
        </Grid>
        <Grid size={{ xs: 12, sm: 6, md: 4 }}>
          <StatCard
            label="Категорий (вкл / всего)"
            value={`${stats.EnabledCategories ?? 0} / ${stats.TotalCategories ?? 0}`}
            icon={<Category fontSize="inherit" />} color="#10b981" />
        </Grid>
        <Grid size={{ xs: 12, sm: 6, md: 4 }}>
          <StatCard label="Замаплено категорий" value={stats.MappedCategories ?? 0} icon={<CheckCircle fontSize="inherit" />} color="#f59e0b" />
        </Grid>
        <Grid size={{ xs: 12, sm: 6, md: 4 }}>
          <StatCard label="Отправлено в CS-Cart" value={stats.PushedProducts ?? 0} icon={<CloudUpload fontSize="inherit" />} color="#3b82f6" />
        </Grid>
        <Grid size={{ xs: 12, sm: 6, md: 4 }}>
          <StatCard label="Ожидают перевода" value={stats.PendingTranslate ?? 0} icon={<Translate fontSize="inherit" />} color="#ec4899" />
        </Grid>
        <Grid size={{ xs: 12, sm: 6, md: 4 }}>
          <StatCard
            label="Последний синк"
            value={stats.LastSync ? ts(stats.LastSync.StartedAt) : '-'}
            icon={<Sync fontSize="inherit" />} color="#64748b" />
        </Grid>
      </Grid>

      <Card>
        <CardContent>
          <Typography sx={{ fontWeight: 600, mb: 2 }}>Последние задачи синхронизации</Typography>
          {!jobs.length && <Typography sx={{ color: 'text.secondary', fontSize: 13 }}>Задач нет.</Typography>}
          {jobs.map((job, i) => (
            <Box key={job.ID}>
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, py: '8px' }}>
                <Chip label={job.Status} color={jobColor(job.Status)} size="small" sx={{ minWidth: 60 }} />
                <Chip label={job.JobType} size="small" variant="outlined" />
                <Typography sx={{ fontSize: 12, flexGrow: 1, color: '#495057' }}>
                  {job.CategoryID || 'все категории'}
                </Typography>
                <Typography sx={{ fontSize: 11, color: 'text.secondary' }}>
                  {job.ItemsProcessed > 0 ? `+${job.ItemsProcessed} тов. · ` : ''}
                  {ts(job.StartedAt)}
                </Typography>
              </Box>
              {i < jobs.length - 1 && <Divider />}
            </Box>
          ))}
        </CardContent>
      </Card>
    </Box>
  )
}
