interface TagManagerEvent {
  /**
   * event is the name of the event, used to filter events in
   * Google Analytics.
   */
  event: string;

  /**
   * event_category is a name that you supply as a way to group objects
   * that to analyze. Typically, you will use the same category name
   * multiple times over related UI elements (buttons, links, etc).
   */
  event_category?: string;

  /**
   * event_action is used to name the type of event or interaction you
   * want to measure for a particular web object. For example, with a
   * single "form" category, you can analyze a number of specific events
   * with this parameter, such as: form entered, form submitted.
   */
  event_action?: string;

  /**
   * event_label provide additional information for events that you want
   * to analyze, such as the text label of a link.
   */
  event_label?: string;

  /**
   * gtm.start is used to initialize Google Tag Manager.
   */
  'gtm.start'?: number;
}

// eslint-disable-next-line @typescript-eslint/no-unused-vars
declare global {
  interface Window {
    dataLayer?: (TagManagerEvent | VoidFunction)[];
    ga?: unknown;
  }
}

/**
 * track sends events to Google Tag Manager.
 */
export function track(
  event: string | TagManagerEvent,
  category?: string,
  action?: string,
  label?: string
): void {
  window.dataLayer ??= [];
  if (typeof event === 'string') {
    window.dataLayer.push({
      event,
      event_category: category,
      event_action: action,
      event_label: label,
    });
  } else {
    window.dataLayer.push(event);
  }
}

/**
 * func adds functions to run sequentionally after
 * Google Tag Manager is ready.
 */
export function func(fn: () => void): void {
  window.dataLayer ??= [];
  window.dataLayer.push(fn);
}
