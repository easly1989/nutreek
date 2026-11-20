// reset.js: a form with data-reset-on-success empties itself once its htmx
// request succeeded and puts the focus back on its [data-reset-focus] field,
// so several entries can be typed in a row. A response with HX-Redirect (an
// error shown after a full page load) leaves the form alone.
document.body.addEventListener("htmx:afterRequest", function (evt) {
  var form = evt.detail.elt;
  if (!form || !form.matches || !form.matches("form[data-reset-on-success]")) return;
  var xhr = evt.detail.xhr;
  if (!evt.detail.successful || (xhr && xhr.getResponseHeader("HX-Redirect"))) return;
  form.reset();
  var field = form.querySelector("[data-reset-focus]");
  if (field) field.focus({ preventScroll: true });
});
