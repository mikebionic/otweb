import { useState, useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  Box, Card, Typography, Button, MenuItem, TextField, Chip,
  Divider, CircularProgress, Alert,
} from '@mui/material'
import { AutoAwesome, VisibilityOff, Visibility } from '@mui/icons-material'
import toast from 'react-hot-toast'
import api from '../api/client'
import type { Category } from '../types'

interface CatFeature {
  feature_id: number
  name: string
  product_count: number
  blacklisted: boolean
}

export default function CategoryFeatures() {
  const [cat, setCat] = useState('')
  const [busy, setBusy] = useState(false)
  const [aiUseful, setAiUseful] = useState<number[] | null>(null)

  const { data: cats = [] } = useQuery<Category[]>({
    queryKey: ['categories', 'for-features'],
    queryFn: () => api.get('/categories', { params: { status: 'enabled' } })
      .then(r => r.data.data?.categories ?? r.data.data ?? []),
  })
  const catOptions = useMemo(
    () => (cats || []).filter(c => (c.LocalCount ?? 0) > 0).sort((a, b) => (b.LocalCount ?? 0) - (a.LocalCount ?? 0)),
    [cats],
  )

  const { data, isLoading, refetch } = useQuery<{ features: CatFeature[]; blacklist_count: number }>({
    queryKey: ['cat-features', cat],
    queryFn: () => api.get(`/categories/${cat}/features`).then(r => r.data.data),
    enabled: !!cat,
  })
  const features = data?.features ?? []

  async function toggle(featureId: number, blacklist: boolean) {
    setBusy(true)
    try {
      await api.post(`/categories/${cat}/features/toggle`, { feature_id: featureId, blacklist })
      await refetch()
    } catch (e: any) {
      toast.error(e?.response?.data?.error || e.message)
    } finally { setBusy(false) }
  }

  async function aiSuggest() {
    if (!cat) return
    setBusy(true)
    try {
      const r = await api.post(`/categories/${cat}/features/suggest`)
      const ids: number[] = r.data.data?.suggested ?? []
      setAiUseful(ids)
      toast.success(`AI считает нужными ${ids.length} характеристик (остальные — кандидаты «убрать»)`)
    } catch (e: any) {
      toast.error('AI: ' + (e?.response?.data?.error || e.message))
    } finally { setBusy(false) }
  }

  return (
    <Box>
      <Typography variant="h5" sx={{ fontWeight: 700, mb: 0.5 }}>Характеристики категорий</Typography>
      <Typography sx={{ color: 'text.secondary', mb: 2, fontSize: 14 }}>
        «Убрать» — характеристика уходит в чёрный список: её не пушим в магазин и не переводим
        (в т.ч. для новых синхронизаций). Данные в базе остаются, можно вернуть. AI подскажет нужные.
      </Typography>

      <Card sx={{ p: 2, mb: 2 }}>
        <TextField
          select fullWidth size="small" label="Категория" value={cat}
          onChange={e => { setCat(e.target.value); setAiUseful(null) }} sx={{ maxWidth: 520 }}
        >
          {catOptions.map(c => (
            <MenuItem key={c.ID} value={c.ID}>
              {c.Path || c.Name} <span style={{ color: '#999', marginLeft: 6 }}>· {c.LocalCount} тов.</span>
            </MenuItem>
          ))}
        </TextField>
      </Card>

      {cat && (
        <Card sx={{ p: 2 }}>
          {isLoading ? (
            <Box sx={{ textAlign: 'center', py: 4 }}><CircularProgress /></Box>
          ) : features.length === 0 ? (
            <Alert severity="info">У товаров этой категории нет распознанных характеристик (или ещё не замаплены).</Alert>
          ) : (
            <>
              <Box sx={{ display: 'flex', gap: 1, mb: 2, flexWrap: 'wrap', alignItems: 'center' }}>
                <Button variant="outlined" startIcon={<AutoAwesome />} onClick={aiSuggest} disabled={busy}>
                  AI рекомендация
                </Button>
                {data && data.blacklist_count > 0 && (
                  <Chip label={`в чёрном списке: ${data.blacklist_count}`} color="default" size="small" />
                )}
                {busy && <CircularProgress size={20} />}
              </Box>
              <Divider sx={{ mb: 1 }} />
              {features.map(f => {
                const aiSays = aiUseful === null ? null : aiUseful.includes(f.feature_id)
                return (
                  <Box key={f.feature_id} sx={{
                    display: 'flex', alignItems: 'center', justifyContent: 'space-between', py: 0.6,
                    opacity: f.blacklisted ? 0.5 : 1,
                  }}>
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, flexWrap: 'wrap' }}>
                      <Typography sx={{ textDecoration: f.blacklisted ? 'line-through' : 'none', fontWeight: 500 }}>
                        {f.name}
                      </Typography>
                      <Typography sx={{ color: '#aaa', fontSize: 12 }}>#{f.feature_id}</Typography>
                      <Chip label={`${f.product_count} тов.`} size="small" variant="outlined" />
                      {aiSays === true && <Chip label="AI: нужна" color="success" size="small" />}
                      {aiSays === false && <Chip label="AI: лишняя" color="warning" size="small" variant="outlined" />}
                      {f.blacklisted && <Chip label="скрыта" color="error" size="small" />}
                    </Box>
                    {f.blacklisted ? (
                      <Button size="small" startIcon={<Visibility />} disabled={busy}
                        onClick={() => toggle(f.feature_id, false)}>Вернуть</Button>
                    ) : (
                      <Button size="small" color="error" startIcon={<VisibilityOff />} disabled={busy}
                        onClick={() => toggle(f.feature_id, true)}>Убрать</Button>
                    )}
                  </Box>
                )
              })}
            </>
          )}
        </Card>
      )}
    </Box>
  )
}
