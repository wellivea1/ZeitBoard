// A blank sleep log to print and fill in by hand, for nights kept on paper.
// It is laid out like the clinician report's chart: one row a day from 6 PM to
// 6 PM, so a filled page and the report read the same way. Nothing personal
// goes into it and nothing is read back from it: the nights are entered in
// Log, or copied into the CSV template, by the person who wrote them.

const rows = 31;
const firstHour = 18;

function hourLabel(hour: number) {
  const clock = hour % 12 === 0 ? 12 : hour % 12;
  return `${clock}${hour < 12 ? "a" : "p"}`;
}

export function blankSleepLogHTML() {
  const hours = Array.from({ length: 24 }, (_, index) => (firstHour + index) % 24);
  const head = hours
    .map((hour) => {
      const edge = hour === 0 ? ' class="midnight"' : hour === 12 ? ' class="noon"' : "";
      return `<th scope="col"${edge}>${hourLabel(hour)}</th>`;
    })
    .join("");
  const cells = hours
    .map((hour) =>
      hour === 0
        ? '<td class="midnight"></td>'
        : hour === 12
          ? '<td class="noon"></td>'
          : "<td></td>",
    )
    .join("");
  const body = Array.from(
    { length: rows },
    () => `<tr><th scope="row" class="date"></th>${cells}<td class="notes"></td></tr>`,
  ).join("\n");

  return `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'">
<title>Sleep log</title>
<style>
  @page { size: landscape; margin: 0.35in; }
  * { box-sizing: border-box; }
  body { margin: 0; color: #221f1a; font: 9pt/1.35 Georgia, "Times New Roman", serif; }
  main { max-width: 10.4in; margin: 0 auto; padding: 0.3in; }
  header { display: flex; flex-wrap: wrap; align-items: baseline; justify-content: space-between; gap: 4pt 24pt; border-bottom: 1pt solid #221f1a; padding-bottom: 5pt; }
  h1 { margin: 0; font-size: 17pt; font-weight: 500; }
  .fields { margin: 0; font-family: Arial, sans-serif; font-size: 8pt; }
  .fields span { display: inline-block; min-width: 1.9in; margin-left: 14pt; border-bottom: 0.5pt solid #221f1a; }
  .how { margin: 6pt 0 8pt; font-size: 9pt; }
  table { width: 100%; border-collapse: collapse; table-layout: fixed; }
  th, td { border: 0.5pt solid #9a958b; }
  thead th { padding: 2pt 0; font-family: Arial, sans-serif; font-size: 6.5pt; font-weight: 600; text-align: left; padding-left: 1.5pt; }
  thead th.date, thead th.notes { text-align: left; }
  col.date { width: 0.62in; }
  col.notes { width: 1.35in; }
  tbody tr { height: 0.19in; }
  tbody td:not(.notes) { background-image: linear-gradient(to right, transparent calc(50% - 0.25pt), #d9d4c9 calc(50% - 0.25pt), #d9d4c9 50%, transparent 50%); }
  .midnight, .noon { border-left: 1.25pt solid #221f1a; }
  footer { margin-top: 7pt; font-family: Arial, sans-serif; font-size: 7.5pt; color: #5d574d; }
</style>
</head>
<body>
<main>
<header>
  <h1>Sleep log</h1>
  <p class="fields">Month<span></span> Time zone<span></span></p>
</header>
<p class="how">One row a day, from 6 PM to 6 PM the next day. Shade the time you were asleep, naps included, to the half hour. Put a ? where you are unsure, and leave a row empty for a day you did not record.</p>
<table>
<colgroup><col class="date">${hours.map(() => "<col>").join("")}<col class="notes"></colgroup>
<thead><tr><th scope="col" class="date">Date</th>${head}<th scope="col" class="notes">Notes</th></tr></thead>
<tbody>
${body}
</tbody>
</table>
<footer>ZeitBoard does not read this page. Enter each night in Log › Sleep › Add a past night, or copy them into the CSV template in Data Sources.</footer>
</main>
</body>
</html>
`;
}

export function downloadBlankSleepLog(): boolean {
  if (
    (typeof navigator !== "undefined" && navigator.userAgent.toLowerCase().includes("jsdom")) ||
    typeof Blob === "undefined" ||
    typeof URL === "undefined" ||
    typeof URL.createObjectURL !== "function" ||
    typeof document === "undefined"
  ) {
    return false;
  }
  const url = URL.createObjectURL(new Blob([blankSleepLogHTML()], { type: "text/html" }));
  const link = document.createElement("a");
  link.href = url;
  link.download = "zeitboard-blank-sleep-log.html";
  document.body.appendChild(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);
  return true;
}
