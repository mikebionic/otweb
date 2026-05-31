import { useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Box, Card, CardContent, Typography, LinearProgress, Stack, Button,
  FormControl, InputLabel, Select, MenuItem, TextField, Chip,
  Table, TableBody, TableCell, TableContainer, TableHead, TableRow,
  Switch, FormControlLabel, Collapse, IconButton,
} from '@mui/material'
import { PlayArrow, ExpandMore, ExpandLess } from '@mui/icons-material'
import toast from 'react-hot-toast'
import api from '../api/client'
import type { SyncJob, Category } from '../types'

function jobColor(s: string): any {
  return s === 'done' ? 'success' : s === 'running' ? 'info' : s === 'error' ? 'error' : 'default'
}
function ts(n: number) {
  if (!n) return '-'
  return new Date(n * 1000).toLocaleString('ru-RU', { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' })
}

function JobLogPanel({ jobId }: { jobId: number }) {
  const [open, setOpen] = useState(false)
  const { data } = useQuery({
    queryKey: ['sync-job', jobId],
    queryFn: () => api.get(`/sync/jobs/${jobId}`).then(r => r.data.data),
    refetchInterval: open ? 2_000 : false,
    enabled: open,
  })
  return (
    <Box>
      <IconButton size="small" onClick={() => setOpen(v => !v)}>
        {open ? <ExpandLess sx={{ fontSize: 14 }} /> : <ExpandMore sx={{ fontSize: 14 }} />}
      </IconButton>
      <Collapse in={open}>
        <Box sx={{ background: '#1a1a2e', color: '#a8ff78', fontFamily: 'monospace', fontSize: 11, p: 1.5, mt: 0.5, borderRadius: 1, maxHeight: 200, overflow: 'auto', whiteSpace: 'pre-wrap' }}>
          {data?.log || 'Лог пуст'}
        </Box>
      </Collapse>
    </Box>
  )
}

export default function Sync() {
  const [searchParams] = useSearchParams()
  const [categoryId, setCategoryId] = useState(searchParams.get('category') ?? '')
  const [maxProducts, setMaxProducts] = useState('500')
  const [minPrice, setMinPrice] = useState('')
  const [maxPrice, setMaxPrice] = useState('')
  const [maxPriceLimit, setMaxPriceLimit] = useState('')
  const [pricesOnly, setPricesOnly] = useState(false)
  const qc = useQueryClient()

  const { data, isLoading } = useQuery({
    queryKey: ['sync'],
    queryFn: () => api.get('/sync').then(r => r.data.data),
    refetchInterval: 5_000,
  })

  const runSync = useMutation({
    mutationFn: () => api.post('/sync/run', {
      category_id: categoryId,
      max_products: parseInt(maxProducts) || 500,
      min_price: parseFloat(minPrice) || 0,
      max_price: parseFloat(maxPrice) || 0,
      max_price_limit: parseFloat(maxPriceLimit) || 0,
      prices_only: pricesOnly,
    }),
    onSuccess: r => { toast.success(`Задача #${r.data.data.job_id} запущена`); qc.invalidateQueries({ queryKey: ['sync'] }) },
    onError: () => toast.error('Ошибка'),
  })

  const jobs: SyncJob[] = data?.jobs ?? []
  const cats: Category[] = data?.categories ?? []

  return (
    <Box>
      <Typography sx={{ fontSize: 20, fontWeight: 700, mb: 2 }}>Синхронизация</Typography>

      <Card sx={{ mb: 3 }}>
        <CardContent>
          <Typography sx={{ fontWeight: 600, mb: 2 }}>Запустить синхронизацию</Typography>
          <Stack spacing={2}>
            <FormControl size="small" fullWidth>
              <InputLabel>Категория</InputLabel>
              <Select value={categoryId} label="Категория" onChange={e => setCategoryId(e.target.value as string)}>
                <MenuItem value="">Все включённые категории</MenuItem>
                {cats.filter(c => c.Enabled).map(c => (
                  <MenuItem key={c.ID} value={c.ID}>{c.Name} <Typography component="span" sx={{ fontSize: 11, color: 'text.secondary', ml: 0.5 }}>({c.ID})</Typography></MenuItem>
                ))}
              </Select>
            </FormControl>
            <Stack direction="row" spacing={2} sx={{ flexWrap: 'wrap' }}>
              <TextField size="small" label="Макс. товаров" value={maxProducts} onChange={e => setMaxProducts(e.target.value)} sx={{ width: 140 }} />
              <TextField size="small" label="Мин. цена CNY" value={minPrice} onChange={e => setMinPrice(e.target.value)} sx={{ width: 140 }} />
              <TextField size="small" label="Макс. цена CNY" value={maxPrice} onChange={e => setMaxPrice(e.target.value)} sx={{ width: 140 }} />
              <TextField size="small" label="Лимит аномалий CNY" value={maxPriceLimit} onChange={e => setMaxPriceLimit(e.target.value)} sx={{ width: 160 }} placeholder="0 = откл" />
            </Stack>
            <FormControlLabel
              control={<Switch checked={pricesOnly} onChange={e => setPricesOnly(e.target.checked)} />}
              label={<Typography sx={{ fontSize: 13 }}>Только цены (без загрузки новых товаров)</Typography>}
            />
            <Box>
              <Button variant="contained" startIcon={<PlayArrow />}
                onClick={() => runSync.mutate()} disabled={runSync.isPending}>
                Запустить
              </Button>
            </Box>
          </Stack>
        </CardContent>
      </Card>

      <Card>
        <CardContent>
          <Typography sx={{ fontWeight: 600, mb: 2 }}>История задач</Typography>
          {isLoading && <LinearProgress />}
          <TableContainer>
            <Table size="small">
              <TableHead>
                <TableRow>
                  <TableCell>#</TableCell>
                  <TableCell>Тип</TableCell>
                  <TableCell>Категория</TableCell>
                  <TableCell>Статус</TableCell>
                  <TableCell align="right">+Товаров</TableCell>
                  <TableCell align="right">Ошибок</TableCell>
                  <TableCell>Время</TableCell>
                  <TableCell>Лог</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {jobs.map(job => (
                  <TableRow key={job.ID} hover>
                    <TableCell sx={{ color: 'text.secondary', fontSize: 12 }}>{job.ID}</TableCell>
                    <TableCell><Chip label={job.JobType} size="small" variant="outlined" /></TableCell>
                    <TableCell><Typography sx={{ fontSize: 12 }}>{job.CategoryID || 'все'}</Typography></TableCell>
                    <TableCell><Chip label={job.Status} color={jobColor(job.Status)} size="small" /></TableCell>
                    <TableCell align="right">
                      <Typography sx={{ fontSize: 12, color: job.ItemsProcessed > 0 ? 'success.main' : 'text.secondary' }}>
                        {job.ItemsProcessed > 0 ? `+${job.ItemsProcessed}` : '-'}
                      </Typography>
                    </TableCell>
                    <TableCell align="right">
                      <Typography sx={{ fontSize: 12, color: job.ErrorsCount > 0 ? 'error.main' : 'text.secondary' }}>
                        {job.ErrorsCount || '-'}
                      </Typography>
                    </TableCell>
                    <TableCell><Typography sx={{ fontSize: 11, color: 'text.secondary' }}>{ts(job.StartedAt)}</Typography></TableCell>
                    <TableCell>
                      <JobLogPanel jobId={job.ID} />
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </TableContainer>
        </CardContent>
      </Card>
    </Box>
  )
}
