import type {
  ColumnMeta as _ColumnMeta,
  FilterFn,
  FilterMeta,
  RankingInfo,
  RowData,
  SortingFn,
} from "@tanstack/table-core";

declare module "@tanstack/table-core" {
  interface FilterFns {
    fuzzy: FilterFn<unknown>;
    dateRange: FilterFn<unknown>;
    exactMatch: FilterFn<unknown>;
    arrIncludesSomeExact: FilterFn<unknown>;
  }
  interface FilterMeta {
    itemRank: RankingInfo;
  }
  interface SortingFns {
    checkbox: SortingFn<unknown>;
    datetime: SortingFn<unknown>;
  }
  interface ColumnMeta<TData extends RowData, TValue> {
    /** Classes applied to both the header and body cells of the column. */
    className?: string;
  }
}
