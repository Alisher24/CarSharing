import type { ReactNode } from 'react';
import { Navigate, Route, Routes } from 'react-router';
import { ACCOUNT_ADDRESS, INVOICES_ADDRESS, INVOICE_PARAMETER, MAP_ADDRESS, RIDES_ADDRESS } from './addresses';
import { MapScreen } from './MapScreen';
import type { Application } from './useApplication';
import { AccountScreen } from '../features/cabinet/AccountScreen';
import { InvoiceCard } from '../features/cabinet/InvoiceCard';
import { InvoiceFeed } from '../features/cabinet/InvoiceFeed';
import { RideFeed } from '../features/cabinet/RideFeed';

type AppRoutesProps = {
  application: Application;

  /** Whether the entry panel above the map is open, which the header's one control decides. */
  entryOpen: boolean;
};

/**
 * AppRoutes is which screen each address shows. The addresses are part of the application's outward
 * behaviour — a person sends a link to one of their invoices and expects to arrive at it — so they
 * are declared once, in one table, and the router only chooses between them.
 *
 * The router reads no data of its own. Reading, freshness and reconciliation belong to the
 * coordinator every screen below already uses, and a second mechanism beside it would disagree with
 * it on the first change signal.
 */
export function AppRoutes({ application, entryOpen }: AppRoutesProps) {
  const { account, submission, submit, leave, privateEvents } = application;

  const cabinet = (section: ReactNode) => (
    <AccountScreen account={account} submission={submission} onSubmit={submit} onLeave={leave}>
      {section}
    </AccountScreen>
  );

  return (
    <Routes>
      <Route path={MAP_ADDRESS} element={<MapScreen application={application} entryOpen={entryOpen} />} />
      <Route path={ACCOUNT_ADDRESS} element={<Navigate to={RIDES_ADDRESS} replace />} />
      <Route path={RIDES_ADDRESS} element={cabinet(<RideFeed account={account} events={privateEvents} />)} />
      <Route path={INVOICES_ADDRESS} element={cabinet(<InvoiceFeed account={account} events={privateEvents} />)} />
      <Route
        path={`${INVOICES_ADDRESS}/:${INVOICE_PARAMETER}`}
        element={cabinet(<InvoiceCard account={account} events={privateEvents} />)}
      />
      {/* An address this application does not serve is the map rather than a screen of its own: there
          is nothing to say about it that the map does not say better. */}
      <Route path="*" element={<Navigate to={MAP_ADDRESS} replace />} />
    </Routes>
  );
}
