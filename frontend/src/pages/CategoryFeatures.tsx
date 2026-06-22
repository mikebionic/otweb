import { useState, useMemo, useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  Box, Card, Typography, Button, Checkbox, FormControlLabel, MenuItem,
  TextField, Chip, Divider, CircularProgress, Alert,
} from '@mui/material'
import { AutoAwesome, Save, DeleteSweep } from '@mui/icons-material'
import toast from 'react-hot-toast'
import api from '../api/client'
import type { Category } from '../types'

interface CatFeature {
  feature_id: number
  name: string
  product_count: number
  in_whitelist: boolean
}

export default function CategoryFeatures() {
  const [cat, setCat] = useState('')
  const [checked, setChecked] = useState<Record<number, boolean>>({})
  const [busy, setBusy] = useState(false)

  // список категорий (только с товарами)
  const { data: cats = [] } = useQuery<Category[]>({
    queryKey: ['categories', 'for-features'],
    queryFn: () => api.get('/categories', { params: { status: 'enabled' } }).then(r => r.data.data),
  })
  const catOptions = useMemo(
    () => (cats || []).filter(c => (c.LocalCount ?? 0) > 0).sort((a, b) => (b.LocalCount ?? 0) - (a.LocalCount ?? 0)),
    [cats],
  )

  // характеристики выбранной категории
  const { data, isLoading, refetch } = useQuery<{ features: CatFeature[]; whitelist_active: boolean }>({
    queryKey: ['cat-features', cat],
    queryFn: () => api.get(`/categories/${cat}/features`).then(r => r.data.data),
    enabled: !!cat,
  })
  const features = data?.features ?? []

  useEffect(() => {
    const init: Record<number, boolean> = {}
    for (const f of features) init[f.feature_id] = f.in_whitelist
    setChecked(init)
  }, [data])

  async function aiSuggest() {
    if (!cat) return
    setBusy(true)
    try {
      const r = await api.post(`/categories/${cat}/features/suggest`)
      const ids: number[] = r.data.data?.suggested ?? []
      const next: Record<number, boolean> = {}
      for (const f of features) next[f.feature_id] = ids.includes(f.feature_id)
      setChecked(next)
      toast.success(`AI отметил ${ids.length} нужных характеристик`)
    } catch (e: any) {
      toast.error('AI: ' + (e?.response?.data?.error || e.message))
    } finally { setBusy(false) }
  }

  async function save() {
    if (!cat) return
    setBusy(true)
    try {
      const feature_ids = Object.entries(checked).filter(([, v]) => v).map(([k]) => Number(k))
      await api.post(`/categories/${cat}/features`, { feature_ids })
      toast.success(`Сохранено: ${feature_ids.length} характеристик. Лишние не будут пушиться.`)
      refetch()
    } catch (e: any) {
      toast.error('Сохранение: ' + (e?.response?.data?.error || e.message))
    } finally { setBusy(false) }
  }

  async function cleanup() {
    if (!cat) return
    if (!confirm('Удалить из базы лишние характеристики (которых нет в списке) для товаров этой категории? Действие необратимо.')) return
    setBusy(true)
    try {
      const r = await api.post(`/categories/${cat}/features/cleanup`)
      toast.success(`Удалено лишних атрибутов: ${r.data.data?.deleted ?? 0}`)
      refetch()
    } catch (e: any) {
      toast.error('Очистка: ' + (e?.response?.data?.error || e.message))
    } finally { setBusy(false) }
  }

  const checkedCount = Object.values(checked).filter(Boolean).length

  return (
    <Box>
      <Typography variant="h5" sx={{ fontWeight: 700, mb: 0.5 }}>Характеристики категорий</Typography>
      <Typography sx={{ color: 'text.secondary', mb: 2, fontSize: 14 }}>
        Утвердите, какие характеристики нужны в категории. Только отмеченные импортируются/пушатся в магазин,
        остальные игнорируются. AI подскажет нужные. «Очистить» удаляет лишние из базы.
      </Typography>

      <Card sx={{ p: 2, mb: 2 }}>
        <TextField
          select fullWidth size="small" label="Категория" value={cat}
          onChange={e => setCat(e.target.value)} sx={{ maxWidth: 520 }}
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
            <Alert severity="info">У товаров этой категории нет распознанных характеристик (или они ещё не замаплены).</Alert>
          ) : (
            <>
              <Box sx={{ display: 'flex', gap: 1, mb: 2, flexWrap: 'wrap', alignItems: 'center' }}>
                <Button variant="outlined" startIcon={<AutoAwesome />} onClick={aiSuggest} disabled={busy}>
                  AI рекомендация
                </Button>
                <Button variant="contained" startIcon={<Save />} onClick={save} disabled={busy}>
                  Сохранить ({checkedCount})
                </Button>
                <Button color="error" variant="outlined" startIcon={<DeleteSweep />} onClick={cleanup} disabled={busy}>
                  Очистить лишнее из БД
                </Button>
                {data?.whitelist_active && <Chip label="whitelist активен" color="success" size="small" />}
                {busy && <CircularProgress size={20} />}
              </Box>
              <Divider sx={{ mb: 1 }} />
              {features.map(f => (
                <Box key={f.feature_id} sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', py: 0.3 }}>
                  <FormControlLabel
                    control={<Checkbox size="small" checked={!!checked[f.feature_id]}
                      onChange={e => setChecked({ ...checked, [f.feature_id]: e.target.checked })} />}
                    label={<span>{f.name} <span style={{ color: '#aaa', fontSize: 12 }}>#{f.feature_id}</span></span>}
                  />
                  <Chip label={`${f.product_count} тов.`} size="small" variant="outlined" />
                </Box>
              ))}
            </>
          )}
        </Card>
      )}
    </Box>
  )
}
