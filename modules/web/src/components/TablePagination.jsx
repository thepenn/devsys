import React from 'react';
import { Select } from 'antd';
import './table-pagination.less';

const DEFAULT_PAGE_SIZE_OPTIONS = [10, 20, 50, 100];

const TablePagination = ({
  page = 1,
  pageSize = 10,
  total = 0,
  pageSizeOptions = DEFAULT_PAGE_SIZE_OPTIONS,
  onChange,
  className = ''
}) => {
  const selectOptions = pageSizeOptions.map(size => ({ value: size, label: size }));
  const safePageSize = pageSize > 0 ? pageSize : 10;
  const totalPages = Math.max(1, Math.ceil((Number(total) || 0) / safePageSize));
  const current = Math.min(Math.max(page || 1, 1), totalPages);

  const triggerChange = (nextPage, nextSize = safePageSize) => {
    if (typeof onChange === 'function') {
      onChange(nextPage, nextSize);
    }
  };

  const handlePrev = () => {
    if (current <= 1) return;
    triggerChange(current - 1);
  };

  const handleNext = () => {
    if (current >= totalPages) return;
    triggerChange(current + 1);
  };

  const handlePageSizeChange = value => {
    if (!value || value === safePageSize) return;
    triggerChange(1, value);
  };

  return (
    <div className={`table-pagination ${className}`.trim()}>
      <div className="table-pagination__left">
        <span className="table-pagination__label">每页显示：</span>
        <Select
          size="small"
          value={safePageSize}
          options={selectOptions}
          onChange={handlePageSizeChange}
          dropdownMatchSelectWidth={false}
        />
        <span className="table-pagination__total">总数：{total || 0}</span>
      </div>
      <div className="table-pagination__right">
        <button
          type="button"
          className="table-pagination__arrow"
          onClick={handlePrev}
          disabled={current <= 1}
        >
          ←
        </button>
        <span className="table-pagination__status">
          {current} / {totalPages}
        </span>
        <button
          type="button"
          className="table-pagination__arrow"
          onClick={handleNext}
          disabled={current >= totalPages}
        >
          →
        </button>
      </div>
    </div>
  );
};

export default TablePagination;
