import { useState, useCallback, useRef } from 'react'
import { search } from './api'

const PAGE_SIZE = 20

export function useSearch() {
  const [query, setQuery]       = useState('')
  const [results, setResults]   = useState([])
  const [total, setTotal]       = useState(0)
  const [timeTaken, setTimeTaken] = useState(null)
  const [loading, setLoading]   = useState(false)
  const [error, setError]       = useState(null)
  const [page, setPage]         = useState(0)
  const [searched, setSearched] = useState(false)
  const abortRef = useRef(null)

  const doSearch = useCallback(async (q, pageNum = 0) => {
    if (!q.trim()) return
    // cancel any in-flight request
    if (abortRef.current) abortRef.current.abort()
    abortRef.current = new AbortController()

    setLoading(true)
    setError(null)
    setSearched(true)

    try {
      const data = await search({ query: q, limit: PAGE_SIZE, offset: pageNum * PAGE_SIZE })
      setResults(data.results || [])
      setTotal(data.total || 0)
      setTimeTaken(data.time_taken_ms)
      setPage(pageNum)
    } catch (e) {
      if (e.name !== 'AbortError') {
        setError(e.message)
        setResults([])
        setTotal(0)
      }
    } finally {
      setLoading(false)
    }
  }, [])

  const handleQueryChange = (q) => {
    setQuery(q)
    if (!q.trim()) {
      setResults([])
      setTotal(0)
      setSearched(false)
      setTimeTaken(null)
    }
  }

  const goToPage = (p) => doSearch(query, p)

  const totalPages = Math.ceil(total / PAGE_SIZE)

  return {
    query, setQuery: handleQueryChange,
    results, total, timeTaken,
    loading, error,
    page, totalPages, goToPage,
    searched,
    doSearch,
  }
}
